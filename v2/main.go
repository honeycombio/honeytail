package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"math/rand"

	flag "github.com/jessevdk/go-flags"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htfilter "github.com/honeycombio/honeytail/v2/filter"
	htfilter_registry "github.com/honeycombio/honeytail/v2/filter/registry"
	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_registry "github.com/honeycombio/honeytail/v2/parser/registry"
	htsource "github.com/honeycombio/honeytail/v2/source"
	htsource_registry "github.com/honeycombio/honeytail/v2/source/registry"
	htuploader "github.com/honeycombio/honeytail/v2/uploader"
	htutil "github.com/honeycombio/honeytail/v2/util"
	"sort"
)

// Set via linker flag "-X" by Travis CI
var BuildID string = ""

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

	version := "dev"
	if BuildID != "" {
		version = BuildID
	}
	// TODO(kannan): The old code would include the parser name.  Do we still need to do that?
	userAgent := fmt.Sprintf("honeytail/%s", version)

	err = startUploader(flags.TestMode, userAgent, mc.uploaderConfig, flags.WriteKeyFile, eventChannel, doneWG)
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

func startParser(config *ParserConfig, filterFactory htfilter.Factory,
	lineChannelChannel <-chan (<-chan string), eventChannel chan<- htevent.Event) error {

	channelSize := 2 * config.numThreads
	closeChannel, preParserFunc, parserFunc, err := config.buildFunc(channelSize)
	if err != nil {
		return fmt.Errorf("building parser: %s", err)
	}

	// Start pre-parser thread for each new line channel.
	go func(){
		wg := sync.WaitGroup{}
		for lineChannel := range lineChannelChannel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				preParserFunc(lineChannel, newSampler(config.preSampleRate))
			}()
		}
		wg.Wait()
		closeChannel()
	}()

	// Start parser threads.
	parserWG := sync.WaitGroup{}
	for i := 0; i < config.numThreads; i++ {
		parserWG.Add(1)
		go func() {
			defer parserWG.Done()
			filterFunc := filterFactory()  // Thread-local, to avoid contention overhead

			// The parser function doesn't get direct access to the event channel.  We instead
			// pass it a 'sendEvent' function so we can apply filtering before putting it in
			// the channel.
			sendEvent := func(timestamp time.Time, data map[string]interface{}) {
				event := htevent.Event{
					SampleRate: config.preSampleRate,
					Timestamp: timestamp,
					Data: data,
				}
				keep := filterFunc(&event)
				if keep {
					eventChannel <- event
				}
			}
			parserFunc(sendEvent)
		}()
	}
	// Close event channel when all parsers threads are done.
	go func() {
		parserWG.Wait()
		close(eventChannel)
	}()

	return nil
}

type dummySampler struct{}

type randSampler struct {
	randObj rand.Rand
	rate uint
}

func newSampler(rate uint) htparser.Sampler {
	if rate < 1 {
		panic(fmt.Sprintf("bad rate: %d", rate))
	}
	if rate == 1 {
		return dummySampler{}
	}
	return randSampler{
		*rand.New(rand.NewSource(rand.Int63())),
		rate,
	}
}

func (_ dummySampler) ShouldKeep() bool {
	return true
}

func (s randSampler) ShouldKeep() bool {
	return s.randObj.Intn(int(s.rate)) == 0
}

func startUploader(testMode bool, userAgent string, uploaderConfig *htuploader.Config,
	writeKeyFilePath string, eventChannel <-chan htevent.Event, doneWG *sync.WaitGroup) error {

	if testMode {
		doneWG.Add(1)
		go func() {
			defer doneWG.Done()
			fmt.Printf("User-Agent: %s\n", userAgent)
			for event := range eventChannel {
				fmt.Printf("[%v] 1/%d\n", event.Timestamp, event.SampleRate)
				for _, k := range sortedKeys(event.Data) {
					fmt.Printf("    %s  %#v\n", k, event.Data[k])
				}
			}
		}()
	} else {
		if writeKeyFilePath == "" {
			return errors.New("missing flag \"--write-key-file\"; to just test parsing, pass \"-test\".")
		}
		if uploaderConfig == nil {
			return errors.New("configuration is missing \"uploader\" section; to just test parsing, pass \"-test\".")
		}
		writeKeyConfig, err := loadWriteKeyConfig(writeKeyFilePath)
		if err != nil {
			return fmt.Errorf("\"--write-key-file\": %s", err)
		}

		htuploader.Start(userAgent, uploaderConfig, writeKeyConfig, eventChannel, doneWG)
	}
	return nil
}

func sortedKeys(m map[string]interface{}) []string {
	var keys []string
	for k, _ := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type MainConfig struct {
	sourceStartFunc htsource.StartFunc
	parserConfig    *ParserConfig
	filterFactory   htfilter.Factory
	uploaderConfig  *htuploader.Config
}

type ParserConfig struct {
	numThreads    int
	preSampleRate uint
	buildFunc     htparser.BuildFunc
}

func ExtractMainConfig(v *sx.Value, backfill bool) *MainConfig{
	r := &MainConfig{}
	v.Map(func(m sx.Map) {
		r.sourceStartFunc = htsource_registry.Build(m.Pop("source"), backfill)

		r.parserConfig = ExtractParserConfig(m.Pop("parser"))

		r.uploaderConfig = nil
		m.PopMaybeAnd("uploader", func(v *sx.Value) {
			r.uploaderConfig = htuploader.ExtractConfig(v, backfill)
		})

		r.filterFactory = htfilter_registry.Build(m.PopMaybe("filter"))
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
			r.preSampleRate = uint(v.UInt32B(1, 1 * 1000 * 1000))
		})

		r.buildFunc = htparser_registry.Configure(m.Pop("engine"))
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
