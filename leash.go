package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/honeycombio/urlshaper"
	"github.com/sirupsen/logrus"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/parsers"
	"github.com/honeycombio/honeytail/parsers/arangodb"
	"github.com/honeycombio/honeytail/parsers/csv"
	"github.com/honeycombio/honeytail/parsers/htjson"
	"github.com/honeycombio/honeytail/parsers/keyval"
	"github.com/honeycombio/honeytail/parsers/mongodb"
	"github.com/honeycombio/honeytail/parsers/mysql"
	"github.com/honeycombio/honeytail/parsers/nginx"
	"github.com/honeycombio/honeytail/parsers/postgresql"
	"github.com/honeycombio/honeytail/parsers/regex"
	"github.com/honeycombio/honeytail/parsers/syslog"
	"github.com/honeycombio/honeytail/sample"
	"github.com/honeycombio/honeytail/tail"
)

const backfillMessage = `Running in backfill mode may result in rate-limited events for this dataset. This is expected behavior.
Be aware that if you are also sending data from other sources to this dataset, this may result in events
being dropped.`
const rateLimitMessageBackoff = `One or more of your events sent by honeytail has been rate-limited, and is being resent. This may result in a
notification to your team administrators that rate-limiting has been triggered on this dataset. While this is
expected behavior, it is possible that if there are other sources sending data to this dataset, that this may
result in events being dropped. 
`

var previouslyRateLimited = false

// actually go and be leashy
func run(ctx context.Context, options GlobalOptions) {
	logrus.Info("Starting honeytail")

	stats := newResponseStats()

	sigs := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(ctx)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// spin up our transmission to send events to Honeycomb
	libhConfig := libhoney.Config{
		WriteKey:             options.Reqs.WriteKey,
		Dataset:              options.Reqs.Dataset,
		APIHost:              options.APIHost,
		MaxConcurrentBatches: options.NumSenders,
		SendFrequency:        time.Duration(options.BatchFrequencyMs) * time.Millisecond,
		MaxBatchSize:         options.BatchSize,
		// block on send should be true so if we can't send fast enough, we slow
		// down reading the log rather than drop lines.
		BlockOnSend: true,
		// block on response is true so that if we hit rate limiting we make sure
		// to re-enqueue all dropped events
		BlockOnResponse: true,

		// limit pending work capacity so that we get backpressure from libhoney
		// and block instead of sleeping inside sendToLibHoney.
		PendingWorkCapacity: 20 * options.NumSenders,
	}
	if options.DebugOut {
		libhConfig.Output = &libhoney.WriterOutput{}
	}
	if err := libhoney.Init(libhConfig); err != nil {
		logrus.WithFields(logrus.Fields{"err": err}).Fatal(
			"Error occurred while spinning up Transmission")
	}

	if options.Backfill {
		logrus.Info(backfillMessage)
	}

	// compile the prefix regex once for use on all channels
	var prefixRegex *parsers.ExtRegexp
	if options.PrefixRegex == "" {
		prefixRegex = nil
	} else {
		prefixRegex = &parsers.ExtRegexp{regexp.MustCompile(options.PrefixRegex)}
	}

	// get our lines channel from which to read log lines
	var linesChans []chan string
	var err error
	tc := tail.Config{
		Paths:       options.Reqs.LogFiles,
		FilterPaths: options.FilterFiles,
		Type:        tail.RotateStyleSyslog,
		Options:     options.Tail,
	}
	if options.TailSample {
		linesChans, err = tail.GetSampledEntries(ctx, tc, options.SampleRate)
	} else {
		linesChans, err = tail.GetEntries(ctx, tc)
	}
	if err != nil {
		logrus.WithFields(logrus.Fields{"err": err}).Fatal(
			"Error occurred while trying to tail logfile")
	}

	// set up our signal handler and support canceling
	go func() {
		sig := <-sigs
		fmt.Fprintf(os.Stderr, "Aborting! Caught signal \"%s\"\n", sig)
		fmt.Fprintf(os.Stderr, "Cleaning up...\n")
		cancel()
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

	// for each channel we got back from tail.GetEntries, spin up a parser.
	parsersWG := sync.WaitGroup{}
	responsesWG := sync.WaitGroup{}
	for _, lines := range linesChans {
		// get our parser
		parser, opts := getParserAndOptions(options)
		if parser == nil {
			logrus.WithFields(logrus.Fields{"parser": options.Reqs.ParserName}).Fatal(
				"Parser not found. Use --list to show valid parsers")
		}

		// and initialize it
		if err := parser.Init(opts); err != nil {
			logrus.Fatalf(
				"Error initializing %s parser module: %v", options.Reqs.ParserName, err)
		}

		// create a channel for sending events into libhoney
		toBeSent := make(chan event.Event, options.NumSenders)
		doneSending := make(chan bool)

		// two channels to handle backing off when rate limited and resending failed
		// send attempts that are recoverable
		toBeResent := make(chan event.Event, 2*options.NumSenders)
		// time in milliseconds to delay the send
		delaySending := make(chan int, 2*options.NumSenders)

		// apply any filters to the events before they get sent
		modifiedToBeSent := modifyEventContents(toBeSent, options)

		realToBeSent := make(chan event.Event, 10*options.NumSenders)
		go func() {
			wg := sync.WaitGroup{}
			for i := uint(0); i < options.NumSenders; i++ {
				wg.Add(1)
				go func() {
					for ev := range modifiedToBeSent {
						realToBeSent <- ev
					}
					wg.Done()
				}()
			}
			wg.Wait()
			close(realToBeSent)
		}()

		// start up the sender. all sources are either sampled when tailing or in-
		// parser, so always tell libhoney events are pre-sampled
		go sendToLibhoney(ctx, realToBeSent, toBeResent, delaySending, doneSending)

		// start a goroutine that reads from responses and logs.
		responses := libhoney.TxResponses()
		responsesWG.Add(1)
		go func() {
			handleResponses(responses, stats, toBeResent, delaySending, options)
			responsesWG.Done()
		}()

		parsersWG.Add(1)
		go func(plines chan string) {
			// ProcessLines won't return until lines is closed
			parser.ProcessLines(plines, toBeSent, prefixRegex)
			// trigger the sending goroutine to finish up
			close(toBeSent)
			// wait for all the events in toBeSent to be handed to libhoney
			<-doneSending
			parsersWG.Done()
		}(lines)
	}
	parsersWG.Wait()
	// tell libhoney to finish up sending events
	libhoney.Close()
	// print out what we've done one last time
	responsesWG.Wait()
	stats.log()
	stats.logFinal()

	// Nothing bad happened, yay
	logrus.Info("Honeytail is all done, goodbye!")
}

// getParserOptions takes a parser name and the global options struct
// it returns the options group for the specified parser
func getParserAndOptions(options GlobalOptions) (parsers.Parser, interface{}) {
	var parser parsers.Parser
	var opts interface{}
	switch options.Reqs.ParserName {
	case "regex":
		parser = &regex.Parser{}
		opts = &options.Regex
		opts.(*regex.Options).NumParsers = int(options.NumSenders)
	case "nginx":
		parser = &nginx.Parser{}
		opts = &options.Nginx
		opts.(*nginx.Options).NumParsers = int(options.NumSenders)
	case "json":
		parser = &htjson.Parser{}
		opts = &options.JSON
		opts.(*htjson.Options).NumParsers = int(options.NumSenders)
	case "keyval":
		parser = &keyval.Parser{}
		opts = &options.KeyVal
		opts.(*keyval.Options).NumParsers = int(options.NumSenders)
	case "mongo", "mongodb":
		parser = &mongodb.Parser{}
		opts = &options.Mongo
		opts.(*mongodb.Options).NumParsers = int(options.NumSenders)
	case "mysql":
		parser = &mysql.Parser{
			SampleRate: int(options.SampleRate),
		}
		opts = &options.MySQL
		opts.(*mysql.Options).NumParsers = int(options.NumSenders)
	case "postgresql":
		opts = &options.PostgreSQL
		parser = &postgresql.Parser{}
	case "arangodb":
		parser = &arangodb.Parser{}
		opts = &options.ArangoDB
	case "csv":
		parser = &csv.Parser{}
		opts = &options.CSV
		opts.(*csv.Options).NumParsers = int(options.NumSenders)
	case "syslog":
		parser = &syslog.Parser{}
		opts = &options.Syslog
		opts.(*syslog.Options).NumParsers = int(options.NumSenders)
	}
	parser, _ = parser.(parsers.Parser)
	return parser, opts
}

// modifyEventContents takes a channel from which it will read events. It
// returns a channel on which it will send the munged events. It is responsible
// for hashing or dropping or adding fields to the events and doing the dynamic
// sampling, if enabled
func modifyEventContents(toBeSent chan event.Event, options GlobalOptions) chan event.Event {
	// parse the addField bit once instead of for every event
	parsedAddFields := map[string]string{}
	for _, addField := range options.AddFields {
		splitField := strings.SplitN(addField, "=", 2)
		if len(splitField) != 2 {
			logrus.WithFields(logrus.Fields{
				"add_field": addField,
			}).Fatal("unable to separate provided field into a key=val pair")
		}
		parsedAddFields[splitField[0]] = splitField[1]
	}
	// do all the advance work for request shaping
	shaper := &requestShaper{}
	if len(options.RequestShape) != 0 {
		shaper.pr = &urlshaper.Parser{}
		if options.ShapePrefix != "" {
			shaper.prefix = options.ShapePrefix + "_"
		}
		for _, rpat := range options.RequestPattern {
			pat := urlshaper.Pattern{Pat: rpat}
			if err := pat.Compile(); err != nil {
				logrus.WithField("request_pattern", rpat).WithError(err).Fatal(
					"Failed to compile provided pattern.")
			}
			shaper.pr.Patterns = append(shaper.pr.Patterns, &pat)
		}
	}
	// initialize the dynamic sampler
	var dynamicSampler dynsampler.Sampler
	if len(options.DynSample) != 0 {
		dynamicSampler = &dynsampler.AvgSampleWithMin{
			GoalSampleRate:    options.GoalSampleRate,
			ClearFrequencySec: options.DynWindowSec,
			MinEventsPerSec:   options.MinSampleRate,
		}
		if err := dynamicSampler.Start(); err != nil {
			logrus.WithField("error", err).Fatal("dynsampler failed to start")
		}
	}

	var deterministicSampler *sample.DeterministicSampler
	if options.DeterministicSample != "" {
		var err error
		deterministicSampler, err = sample.NewDeterministicSampler(options.SampleRate)
		if err != nil {
			logrus.WithField("error", err).Fatal("error creating deterministic sampler")
		}
	}

	// initialize the data augmentation map
	// map contents are sourceFieldValue -> object containing new keys and values
	// {"sourceField":{"val1":{"newKey1":"newVal1","newKey2":"newVal2"},"val2":{"newKey1":"newValA"}}}
	type DataAugmentationMap map[string]map[string]map[string]interface{}
	var daMap DataAugmentationMap
	if options.DAMapFile != "" {
		raw, err := ioutil.ReadFile(options.DAMapFile)
		if err != nil {
			logrus.WithField("error", err).Fatal("failed to read Data Augmentation Map file")
		}
		err = json.Unmarshal(raw, &daMap)
		if err != nil {
			logrus.WithField("error", err).Fatal("failed to unmarshal Data Augmentation Map from JSON")
		}
	}

	var baseTime, startTime time.Time
	if options.RebaseTime {
		var err error
		baseTime, err = getBaseTime(options)
		if err != nil {
			logrus.WithError(err).Fatal("--rebase_time specified but cannot rebase")
		}
		startTime = time.Now()
	}

	// ok, we need to munge events. Sing up enough goroutines to handle this
	newSent := make(chan event.Event, options.NumSenders)
	go func() {
		wg := sync.WaitGroup{}
		for i := uint(0); i < options.NumSenders; i++ {
			wg.Add(1)
			go func() {
				for ev := range toBeSent {
					// do request shaping
					for _, field := range options.RequestShape {
						shaper.requestShape(field, &ev, options)
					}
					// do data augmentation. For each source column
					for sourceField, augmentableVals := range daMap {
						// does that column exist in the event?
						if val, ok := ev.Data[sourceField]; ok {
							// if it does exist, is it a string?
							if val, ok := val.(string); ok {
								// if we have fields to augment this value
								if newFields, ok := augmentableVals[val]; ok {
									// go ahead and insert new fields
									for k, v := range newFields {
										ev.Data[k] = v
									}
								}
							}
						}
					}
					// do dropping
					for _, field := range options.DropFields {
						delete(ev.Data, field)
					}
					// do scrubbing
					for _, field := range options.ScrubFields {
						if val, ok := ev.Data[field]; ok {
							// generate a sha256 hash and use the base16 for the content
							newVal := sha256.Sum256([]byte(fmt.Sprintf("%v", val)))
							ev.Data[field] = fmt.Sprintf("%x", newVal)
						}
					}
					// do adding
					for k, v := range parsedAddFields {
						ev.Data[k] = v
					}
					// get presampled field if it exists
					if options.PreSampledField != "" {
						var presampledRate int
						if psr, ok := ev.Data[options.PreSampledField]; ok {
							switch psr := psr.(type) {
							case float64:
								presampledRate = int(psr)
							case string:
								if val, err := strconv.Atoi(psr); err == nil {
									presampledRate = val
								}
							}
						}
						ev.SampleRate = presampledRate
					} else {
						// do sampling
						ev.SampleRate = int(options.SampleRate)
						if dynamicSampler != nil {
							key := makeDynsampleKey(&ev, options)
							sr := dynamicSampler.GetSampleRate(key)
							if rand.Intn(sr) != 0 {
								ev.SampleRate = -1
							} else {
								ev.SampleRate = sr
							}
						}
						if deterministicSampler != nil {
							sampleKey, ok := ev.Data[options.DeterministicSample].(string)
							if !ok {
								logrus.WithField("event_data", ev.Data).
									WithField("field", options.DeterministicSample).
									Error("Field to deterministically sample on does not exist in event, leaving it to random chance")
								if rand.Intn(int(options.SampleRate)) != 0 {
									ev.SampleRate = -1
								}
							} else {
								if !deterministicSampler.Sample(sampleKey) {
									ev.SampleRate = -1
								}
							}
						}
					}
					if options.RebaseTime {
						ev.Timestamp = rebaseTime(baseTime, startTime, ev.Timestamp)
					}
					if len(options.JSONFields) > 0 {
						for _, field := range options.JSONFields {
							jsonVal, ok := ev.Data[field].(string)
							if !ok {
								logrus.WithField("field", field).
									Warn("Error asserting given field as string")
								continue
							}
							var jsonMap map[string]interface{}
							if err := json.Unmarshal([]byte(jsonVal), &jsonMap); err != nil {
								logrus.WithField("field", field).
									Warn("Error unmarshalling field as JSON")
								continue
							}

							ev.Data[field] = jsonMap
						}
					}
					if len(options.RenameFields) > 0 {
						for _, kv := range options.RenameFields {
							kvPair := strings.Split(kv, "=")
							if len(kvPair) != 2 {
								logrus.WithField("arg", kv).
									Error("Invalid --rename_field arg. Should be format 'before=after' ")
								continue
							}
							val, ok := ev.Data[kvPair[0]]
							if !ok {
								logrus.WithField("before_field", kvPair[0]).
									WithField("after_field", kvPair[1]).
									Error("Did not find before_field in event.")
								continue
							}
							delete(ev.Data, kvPair[0])
							ev.Data[kvPair[1]] = val
						}
					}
					newSent <- ev
				}
				wg.Done()
			}()
		}
		wg.Wait()
		close(newSent)
	}()
	return newSent
}

// makeDynsampleKey pulls in all the values necessary from the event to create a
// key for dynamic sampling
func makeDynsampleKey(ev *event.Event, options GlobalOptions) string {
	key := make([]string, len(options.DynSample))
	for i, field := range options.DynSample {
		if val, ok := ev.Data[field]; ok {
			switch val := val.(type) {
			case bool:
				key[i] = strconv.FormatBool(val)
			case int64:
				key[i] = strconv.FormatInt(val, 10)
			case float64:

				key[i] = strconv.FormatFloat(val, 'E', -1, 64)
			case string:
				key[i] = val
			default:
				key[i] = "" // skip it
			}
		}
	}
	return strings.Join(key, "_")
}

// requestShaper holds the bits about request shaping that want to be
// precompiled instead of compute on every event
type requestShaper struct {
	prefix string
	pr     *urlshaper.Parser
}

// requestShape expects the field passed in to have the form
// VERB /path/of/request HTTP/1.x
// If it does, it will break it apart into components, normalize the URL,
// and add a handful of additional fields based on what it finds.
func (r *requestShaper) requestShape(field string, ev *event.Event,
	options GlobalOptions) {
	if val, ok := ev.Data[field]; ok {
		// start by splitting out method, uri, and version
		strval, ok := val.(string)
		if !ok {
			logrus.WithFields(logrus.Fields{
				"value": val,
				"field": field,
				"event": *ev,
			}).Error("Error! Value did not correctly assert to be type string in request shaping. Skipping shaping.")
			return
		}
		parts := strings.Split(strval, " ")
		var path string
		if len(parts) == 3 {
			// treat it as METHOD /path HTTP/1.X
			ev.Data[r.prefix+field+"_method"] = parts[0]
			ev.Data[r.prefix+field+"_protocol_version"] = parts[2]
			path = parts[1]
		} else {
			// treat it as just the /path
			path = parts[0]
		}
		// next up, get all the goodies out of the path
		res, err := r.pr.Parse(path)
		if err != nil {
			// couldn't parse it, just pass along the event
			return
		}
		ev.Data[r.prefix+field+"_uri"] = res.URI
		ev.Data[r.prefix+field+"_path"] = res.Path
		if res.Query != "" {
			ev.Data[r.prefix+field+"_query"] = res.Query
		}
		for k, v := range res.QueryFields {
			// only include the keys we want
			if options.RequestParseQuery == "all" ||
				whitelistKey(options.RequestQueryKeys, k) {
				if len(v) > 1 {
					sort.Strings(v)
				}
				ev.Data[r.prefix+field+"_query_"+k] = strings.Join(v, ", ")
			}
		}
		for k, v := range res.PathFields {
			ev.Data[r.prefix+field+"_path_"+k] = v[0]
		}
		ev.Data[r.prefix+field+"_shape"] = res.Shape
		ev.Data[r.prefix+field+"_pathshape"] = res.PathShape
		if res.QueryShape != "" {
			ev.Data[r.prefix+field+"_queryshape"] = res.QueryShape
		}
	}
}

// return true if the key is in the whitelist
func whitelistKey(whiteKeys []string, key string) bool {
	for _, whiteKey := range whiteKeys {
		if key == whiteKey {
			return true
		}
	}
	return false
}

// sendToLibhoney reads from the toBeSent channel and shoves the events into
// libhoney events, sending them on their way.
func sendToLibhoney(ctx context.Context, toBeSent chan event.Event, toBeResent chan event.Event,
	delaySending chan int, doneSending chan bool) {
	for {
		// check and see if we need to back off the API because of rate limiting
		select {
		case delay := <-delaySending:
			time.Sleep(time.Duration(delay) * time.Millisecond)
		default:
		}
		// if we have events to retransmit, send those first
		select {
		case ev := <-toBeResent:
			// retransmitted events have already been sampled; always use
			// SendPresampled() for these
			sendEvent(ev)
			continue
		default:
		}
		// otherwise pick something up off the regular queue and send it
		select {
		case ev, ok := <-toBeSent:
			if !ok {
				// channel is closed
				// NOTE: any untransmitted retransmittable events will be dropped
				doneSending <- true
				return
			}
			sendEvent(ev)
			continue
		default:
		}
		// no events at all? chill for a sec until we get the next one
		time.Sleep(100 * time.Millisecond)
	}
}

// sendEvent does the actual handoff to libhoney
func sendEvent(ev event.Event) {
	if ev.SampleRate == -1 {
		// drop the event!
		logrus.WithFields(logrus.Fields{
			"event": ev,
		}).Debug("dropped event due to sampling")
		return
	}
	libhEv := libhoney.NewEvent()
	libhEv.Metadata = ev
	libhEv.Timestamp = ev.Timestamp
	libhEv.SampleRate = uint(ev.SampleRate)
	if err := libhEv.Add(ev.Data); err != nil {
		logrus.WithFields(logrus.Fields{
			"event": ev,
			"error": err,
		}).Error("Unexpected error adding data to libhoney event")
	}
	if err := libhEv.SendPresampled(); err != nil {
		logrus.WithFields(logrus.Fields{
			"event": ev,
			"error": err,
		}).Error("Unexpected error event to libhoney send")
	}
}

// handleResponses reads from the response queue, logging a summary and debug
// re-enqueues any events that failed to send in a retryable way
func handleResponses(responses chan transmission.Response, stats *responseStats,
	toBeResent chan event.Event, delaySending chan int,
	options GlobalOptions) {
	go logStats(stats, options.StatusInterval)

	for rsp := range responses {
		stats.update(rsp)
		logfields := logrus.Fields{
			"status_code": rsp.StatusCode,
			"body":        strings.TrimSpace(string(rsp.Body)),
			"duration":    rsp.Duration,
			"error":       rsp.Err,
			"timestamp":   rsp.Metadata.(event.Event).Timestamp,
		}
		// if this is an error we should retry sending, re-enqueue the event
		if options.BackOff && (rsp.StatusCode == 429 || rsp.StatusCode == 500) {
			if !previouslyRateLimited && rsp.StatusCode == 429 {
				logrus.Info(rateLimitMessageBackoff)
				previouslyRateLimited = true
			}
			logfields["retry_send"] = true
			delaySending <- 1000 / int(options.NumSenders) // back off for a little bit
			toBeResent <- rsp.Metadata.(event.Event)       // then retry sending the event
		} else {
			logfields["retry_send"] = false
		}
		logrus.WithFields(logfields).Debug("event send record received")
	}
}

// logStats dumps and resets the stats once every minute
func logStats(stats *responseStats, interval uint) {
	logrus.Debugf("Initializing stats reporting. Will print stats once/%d seconds", interval)
	if interval == 0 {
		// interval of 0 means don't print summary status
		return
	}
	ticker := time.NewTicker(time.Second * time.Duration(interval))
	for range ticker.C {
		stats.logAndReset()
	}
}

func getBaseTime(options GlobalOptions) (time.Time, error) {
	var baseTime time.Time

	// support multiple files and globs, although this is unlikely to be used
	searchFiles := []string{}
	for _, f := range options.Reqs.LogFiles {
		// can't work with stdin
		if f == "-" {
			continue
		}
		// can't work with files that don't exist
		if files, err := filepath.Glob(f); err == nil && files != nil {
			searchFiles = append(searchFiles, files...)
		}
	}
	if len(searchFiles) == 0 {
		return baseTime, fmt.Errorf("unable to get base time, no files found")
	}

	// we're going to have to parse lines, so get an instance of the parser
	parser, parserOpts := getParserAndOptions(options)
	parser.Init(parserOpts)
	lines := make(chan string)
	events := make(chan event.Event)
	var prefixRegex *parsers.ExtRegexp
	if options.PrefixRegex == "" {
		prefixRegex = nil
	} else {
		prefixRegex = &parsers.ExtRegexp{regexp.MustCompile(options.PrefixRegex)}
	}
	// read each file, throw the last line on the lines channel
	go getEndLines(searchFiles, lines)
	// the parser will parse each line and give us an event
	go func() {
		// ProcessLines will stop when the lines channel closes
		parser.ProcessLines(lines, events, prefixRegex)
		// Signal that we're done sending events
		close(events)
	}()

	// we read the event and find the latest timestamp
	// this is our base time (assuming the input files are sorted by time,
	// otherwise we'd have to parse *everything*)
	for ev := range events {
		if ev.Timestamp.After(baseTime) {
			baseTime = ev.Timestamp
		}
	}

	return baseTime, nil
}

func getEndLines(files []string, lines chan<- string) {
	for _, f := range files {
		lines <- getEndLine(f)
	}

	close(lines)
}

func getEndLine(file string) string {
	handle, err := os.Open(file)
	if err != nil {
		logrus.WithError(err).WithField("file", file).
			Fatal("unable to open file")
	}
	defer handle.Close()

	info, err := os.Stat(file)
	if err != nil {
		logrus.WithError(err).WithField("file", file).
			Fatal("unable to stat file")
	}
	// If we're bigger than 2m, zoom to the end of the file and go back 1mb
	// 2m is an arbitrary limit
	if info.Size() > 2*1024*1024 {
		_, err := handle.Seek(-1024*1024, io.SeekEnd)
		if err != nil {
			logrus.WithError(err).WithField("file", file).
				Fatal("unable to seek to last megabyte of file")
		}
	}

	// we use a scanner to read to the last line
	// not the most efficient
	scanner := bufio.NewScanner(handle)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
	}

	if scanner.Err() != nil {
		logrus.WithError(err).WithField("file", file).
			Fatal("unable to read to end of file")
	}

	return line
}

func rebaseTime(baseTime, startTime, timestamp time.Time) time.Time {
	// Figure out the gap between the event and the end of our event window
	delta := baseTime.UnixNano() - timestamp.UnixNano()
	// Create a new time relative to the current time
	return startTime.Add(time.Duration(delta) * time.Duration(-1))
}
