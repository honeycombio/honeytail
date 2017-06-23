package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	flag "github.com/jessevdk/go-flags"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htfilter "github.com/honeycombio/honeytail/v2/filter"
	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_registry "github.com/honeycombio/honeytail/v2/parser/registry"
	htsource "github.com/honeycombio/honeytail/v2/source"
	htsource_registry "github.com/honeycombio/honeytail/v2/source/registry"
	htuploader "github.com/honeycombio/honeytail/v2/uploader"
	htutil "github.com/honeycombio/honeytail/v2/util"
	"strings"
)

func main() {
	var err error

	flags, err := parseFlags(os.Args[1:])
	if err != nil {
		die(2, "%s", err)
	}

	mc, err := loadMainConfig(flags.ConfigFile, flags.Backfill)
	if err != nil {
		die(2, "\"--config-file\": %s", err)
	}

	lineChannelChannel := make(chan (<-chan string))
	eventChannel := make(chan htevent.Event)
	doneWG := &sync.WaitGroup{}
	abort := make(chan struct{})

	setupSignalHandler(abort)

	err = startParser(mc.parserConfig, mc.filterFactory, lineChannelChannel, eventChannel)
	if err != nil {
		die(2, "%s", err)
	}

	err = startUploader(mc.uploaderConfig, flags.TestMode, flags.WriteKeyFile, eventChannel, doneWG)
	if err != nil {
		die(2, "%s", err)
	}

	err = mc.sourceStartFunc(lineChannelChannel, abort)
	if err != nil {
		die(2, "running source: %s", err)
	}

	doneWG.Wait()
}

func die(exitCode int, format string, args ...interface{}) {
	fmt.Fprint(os.Stderr, "Error: ")
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprint(os.Stderr, "\n")
	os.Exit(exitCode)
}

type Flags struct {
	ConfigFile   string `short:"c" long:"config-file" description:"Primary config; see https://github.com/honeycombio/honeytail/v2/config.md"`
	WriteKeyFile string `long:"write-key-file" description:"JSON/JSON5 file with \"write_key\" field."`
	TestMode     bool `long:"test" description:"Don't upload to Honeycomb server; print to stdout instead."`
	Backfill     bool `long:"backfill" description:"Start from the beginning of the log files and don't keep watching."`
}

func parseFlags(args []string) (*Flags, error) {
	var flags *Flags = &Flags{}
	extraArgs, err := flag.NewParser(flags, 0).ParseArgs(args)

	if err != nil {
		return nil, err
	}
	if len(extraArgs) > 0 {
		return nil, fmt.Errorf("unexpected extra arguments: %#v.", extraArgs)
	}
	if flags.ConfigFile == "" {
		return nil, errors.New("missing argument \"-config-file\".")
	}

	return flags, nil
}

func setupSignalHandler(abort chan<- struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		fmt.Fprintf(os.Stderr, "Aborting! Caught signal \"%s\"\n", sig)
		fmt.Fprintf(os.Stderr, "Cleaning up...\n")
		close(abort)
		// and if they insist, catch a second CTRL-C or timeout on 10sec
		select {
		case <-sigs:
			fmt.Fprintf(os.Stderr, "Caught second signal... Aborting.\n")
			os.Exit(1)
		case <-time.After(10 * time.Second):
			fmt.Fprintf(os.Stderr, "Taking too long... Aborting.\n")
			os.Exit(1)
		}
	}()
}

func startParser(parserConfig *ParserConfig, filterFactory htfilter.FilterFactory,
	lineChannelChannel <-chan (<-chan string), eventChannel chan<- htevent.Event) error {

	spawnWorkersCalled := false

	// The function that a parser should call to spawn worker threads.  Takes care of spawning
	// the right number of threads and doing post-parse event filtering/sampling.
	workerWG := sync.WaitGroup{}
	spawnWorkers := func(worker htparser.Worker) {
		if spawnWorkersCalled {
			panic("Don't call spawnWorkers more than once!")  // Maybe not necessary?  Should revisit.
		}
		spawnWorkersCalled = true
		for i := 0; i < parserConfig.numThreads; i++ {
			workerWG.Add(1)
			go func() {
				defer workerWG.Done()
				filterFunc := filterFactory()  // Thread-local, to avoid contention overhead
				// We pass 'sendEvent' to the worker so it can send events.
				sendEvent := func(timestamp time.Time, data map[string]interface{}) {
					event := htevent.Event{
						SampleRate: parserConfig.preSampleRate,
						Timestamp: timestamp,
						Data: data,
					}
					keep := filterFunc(&event)
					if keep {
						eventChannel <- event
					}
				}
				worker(sendEvent)
			}()
		}
		// Close channel when all workers are done.
		go func() {
			workerWG.Wait()
			close(eventChannel)
		}()
	}

	err := parserConfig.startFunc(lineChannelChannel, parserConfig.preSampleRate, spawnWorkers)
	if err != nil {
		return fmt.Errorf("running parser: %s", err)
	}
	if !spawnWorkersCalled {
		panic("spawnWorkers was never called; the parser implementation has a bug")
	}

	return nil
}

func startUploader(uploaderConfig *htuploader.Config, testMode bool, writeKeyFilePath string,
	eventChannel <-chan htevent.Event, doneWG *sync.WaitGroup) error {

	if testMode {
		doneWG.Add(1)
		go func() {
			defer doneWG.Done()
			for event := range eventChannel {
				var kvs []string
				for k, v := range event.Data {
					kvs = append(kvs, fmt.Sprintf("%q=%#v", k, v))
				}
				fmt.Printf("[%v] 1/%d %s\n", event.Timestamp, event.SampleRate, strings.Join(kvs, ", "))
			}
		}()
	} else {
		if writeKeyFilePath == "" {
			return errors.New("missing flag \"--write-key-file\"; to just test parsing, pass \"-test\".")
		}
		if uploaderConfig == nil {
			return errors.New("missing \"uploader\" configuration; to just test parsing, pass \"-test\".")
		}
		writeKeyConfig, err := loadWriteKeyConfig(writeKeyFilePath)
		if err != nil {
			return fmt.Errorf("\"--write-key-file\": %s", err)
		}

		htuploader.StartUploader(uploaderConfig, eventChannel, writeKeyConfig, doneWG)
	}
	return nil
}

type MainConfig struct {
	sourceStartFunc htsource.StartFunc
	parserConfig    *ParserConfig
	filterFactory   htfilter.FilterFactory
	uploaderConfig  *htuploader.Config
}

type ParserConfig struct {
	numThreads int
	preSampleRate int
	startFunc htparser.StartFunc
}

func ExtractMainConfig(v *sx.Value, backfill bool) *MainConfig{
	r := &MainConfig{}
	v.Map(func(m sx.Map) {
		r.sourceStartFunc = htsource_registry.Build(m.Pop("source"), backfill)

		r.parserConfig = ExtractParserConfig(m.Pop("parser"))

		r.uploaderConfig = nil
		m.PopMaybeAnd("uploader", func(v *sx.Value) {
			r.uploaderConfig = htuploader.ExtractConfig(v)
		})

		r.filterFactory = htfilter.Build(m.PopMaybe("filter"))
	})
	return r
}

func ExtractParserConfig(v *sx.Value) *ParserConfig {
	r := &ParserConfig{}
	v.Map(func(m sx.Map) {
		r.numThreads = 1
		m.PopMaybeAnd("num_threads", func(v *sx.Value) {
			r.numThreads = int(v.Int32B(1, 1024))
		})

		r.preSampleRate = 1
		m.PopMaybeAnd("pre_sample_rate", func(v *sx.Value) {
			r.preSampleRate = int(v.Int32B(1, 1 * 1000 * 1000))
		})

		r.startFunc = htparser_registry.Build(m.Pop("engine"))
	})
	return r
}

func loadMainConfig(path string, backfill bool) (*MainConfig, error) {
	var err error

	var r *MainConfig
	err = htutil.LoadTomlFileAndExtract(path, func(v *sx.Value) {
		r = ExtractMainConfig(v, backfill)
	})
	if err != nil {
		return nil, err
	}

	return r, nil
}

func loadWriteKeyConfig(path string) (*htuploader.WriteKeyConfig, error) {
	var err error

	var r *htuploader.WriteKeyConfig
	err = htutil.LoadTomlFileAndExtract(path, func(v *sx.Value) {
		r = htuploader.ExtractWriteKeyConfig(v)
	})
	if err != nil {
		return nil, err
	}

	return r, nil
}
