package uploader

import (
	"strings"
	"time"

	htevent "github.com/honeycombio/honeytail/v2/event"
	"github.com/honeycombio/libhoney-go"
	"github.com/Sirupsen/logrus"
	"sync"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"
	"fmt"
	"net/http"
	"io/ioutil"
)

type WriteKeyConfig struct {
	WriteKey string
	ApiUrl string
}

type Config struct {
	dataSet string
	lhc *LibhoneyConfig

	// Whether we should retry sending data if we a rate limit response or 500 response.
	retrySends bool
}

type LibhoneyConfig struct {
	MaxConcurrentBatches uint
	SendFrequency time.Duration
	MaxBatchSize uint
}

func ExtractLibhoneyConfig(v *sx.Value) *LibhoneyConfig {
	// Passing in zeroes to libhoney causes it to use the defaults.
	r := &LibhoneyConfig{
		MaxConcurrentBatches: 0,
		SendFrequency: 0,
		MaxBatchSize: 0,
	}

	if (v != nil) {
		v.Map(func(m sx.Map) {
			m.PopMaybeAnd("max_concurrent_batches", func(v *sx.Value) {
				r.MaxConcurrentBatches = uint(v.UInt32B(1, 1000))
			})
			m.PopMaybeAnd("send_frequency_ms", func(v *sx.Value) {
				r.SendFrequency = time.Duration(v.UInt32B(1, 10 * 1000)) * time.Millisecond
			})
			m.PopMaybeAnd("max_batch_size", func(v *sx.Value) {
				r.MaxBatchSize = uint(v.UInt32B(1, 10000))
			})
		})
	}

	return r
}

func ExtractConfig(v *sx.Value, backfill bool) *Config {
	r := &Config{}
	v.Map(func(m sx.Map) {
		r.dataSet = m.Pop("data_set").String()
		r.lhc = ExtractLibhoneyConfig(m.PopMaybe("libhoney_config"))

		// If we're not backfilling, then a failed send probably means data is coming in
		// at a faster rate than we can upload, so it's probably not worth retrying.
		r.retrySends = backfill
	})
	return r
}

func ExtractWriteKeyConfig(v *sx.Value) *WriteKeyConfig {
	r := &WriteKeyConfig{}
	v.Map(func(m sx.Map) {
		r.WriteKey = m.Pop("write_key").String()

		r.ApiUrl = "https://api.honeycomb.io/"
		m.PopMaybeAnd("api_url", func(v *sx.Value) {
			s := v.String()
			if !strings.HasPrefix(s, "https://") {
				// Should we also allow "http://"?
				v.Fail("must begin with \"https://\"")
			}
			if !strings.HasSuffix(s, "/") {
				v.Fail("must end with \"/\"")
			}
			r.ApiUrl = s
		})
	})
	return r
}


func Start(userAgent string, config *Config, writeKeyConfig *WriteKeyConfig,
	eventChannel <-chan htevent.Event, doneWG *sync.WaitGroup) error {

	err := verifyWriteKeyConfig(userAgent, writeKeyConfig)
	if err != nil {
		return err
	}

	stats := newResponseStats()

	// spin up our transmission to send events to Honeycomb
	libhoney.UserAgentAddition = userAgent
	libhConfig := libhoney.Config{
		WriteKey:             writeKeyConfig.WriteKey,
		Dataset:              config.dataSet,
		APIHost:              writeKeyConfig.ApiUrl,
		MaxConcurrentBatches: config.lhc.MaxConcurrentBatches,
		SendFrequency:        config.lhc.SendFrequency,
		MaxBatchSize:         config.lhc.MaxBatchSize,
		// block on send should be true so if we can't send fast enough, we slow
		// down reading the log rather than drop lines.
		BlockOnSend: true,
		// block on response is true so that if we hit rate limiting we make sure
		// to re-enqueue all dropped events
		BlockOnResponse: true,

		// limit pending work capacity so that we get backpressure from libhoney
		// and block instead of sleeping inside sendToLibHoney.
		PendingWorkCapacity: 20 * config.lhc.MaxConcurrentBatches,
	}
	if err := libhoney.Init(libhConfig); err != nil {
		logrus.WithFields(logrus.Fields{"err": err}).Fatal(
			"Error occured while spinning up Transimission")
	}

	// create a channel for sending events into libhoney
	doneSending := make(chan bool)

	// two channels to handle backing off when rate limited and resending failed
	// send attempts that are recoverable
	toBeResent := make(chan htevent.Event, 2 * config.lhc.MaxConcurrentBatches)
	// time in milliseconds to delay the send
	delaySending := make(chan int, 2 * config.lhc.MaxConcurrentBatches)

	// start up the sender. all sources are either sampled when tailing or in-
	// parser, so always tell libhoney events are pre-sampled
	go sendToLibhoney(eventChannel, toBeResent, delaySending, doneSending)

	// start a goroutine that reads from responses and logs.
	doneWG.Add(1)
	go func() {
		handleResponses(libhoney.Responses(), stats, toBeResent, delaySending, config.retrySends)
		stats.log()
		stats.logFinal()
		doneWG.Done()
	}()

	return nil
}

// sendToLibhoney reads from the toBeSent channel and shoves the events into
// libhoney events, sending them on their way.
func sendToLibhoney(toBeSent <-chan htevent.Event, toBeResent <-chan htevent.Event,
	delaySending <-chan int, doneSending chan<- bool) {

	defer libhoney.Close()
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
func sendEvent(ev htevent.Event) {
	libhEv := libhoney.NewEvent()
	libhEv.Metadata = ev
	libhEv.Timestamp = ev.Timestamp
	libhEv.SampleRate = ev.SampleRate
	if err := libhEv.Add(ev.Data); err != nil {
		logrus.WithFields(logrus.Fields{
			"event": ev,
			"error": err,
		}).Error("Unexpected error adding data to libhoney event")
	}
	// We do the sampling ourselves, so use SendPresampled
	if err := libhEv.SendPresampled(); err != nil {
		logrus.WithFields(logrus.Fields{
			"event": ev,
			"error": err,
		}).Error("Unexpected error event to libhoney send")
	}
}

// handleResponses reads from the response queue, logging a summary and debug
// re-enqueues any events that failed to send in a retryable way
func handleResponses(responses <-chan libhoney.Response, stats *responseStats,
	toBeResent chan<- htevent.Event, delaySending chan int, retrySends bool) {
	statusInterval := uint(1000)  // TODO: allow specifying in config file
	go logStats(stats, statusInterval)

	for rsp := range responses {
		stats.update(rsp)
		logfields := logrus.Fields{
			"status_code": rsp.StatusCode,
			"body":        strings.TrimSpace(string(rsp.Body)),
			"duration":    rsp.Duration,
			"error":       rsp.Err,
			"timestamp":   rsp.Metadata.(htevent.Event).Timestamp,
		}
		// if this is an error we should retry sending, re-enqueue the event
		if retrySends && (rsp.StatusCode == 429 || rsp.StatusCode == 500) {
			logfields["retry_send"] = true
			delaySending <- 1000  // back off for a little bit
			toBeResent <- rsp.Metadata.(htevent.Event)  // then retry sending the event
		} else {
			logfields["retry_send"] = false
		}
		logrus.WithFields(logfields).Debug("event send record received")
	}
}

type responseStats struct {
	lock *sync.Mutex

	count       int
	statusCodes map[int]int
	bodies      map[string]int
	errors      map[string]int
	maxDuration time.Duration
	sumDuration time.Duration
	minDuration time.Duration
	event       *htevent.Event

	totalCount       int
	totalStatusCodes map[int]int
}

// newResponseStats initializes the struct's complex data types
func newResponseStats() *responseStats {
	r := &responseStats{}
	r.totalStatusCodes = make(map[int]int)
	r.lock = &sync.Mutex{}
	r.reset()
	return r
}

// update adds a response into the stats container
func (r *responseStats) update(rsp libhoney.Response) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.count += 1
	r.statusCodes[rsp.StatusCode] += 1
	r.bodies[strings.TrimSpace(string(rsp.Body))] += 1
	if rsp.Err != nil {
		r.errors[rsp.Err.Error()] += 1
	}
	if r.minDuration == 0 {
		r.minDuration = rsp.Duration
	}
	if rsp.Duration < r.minDuration {
		r.minDuration = rsp.Duration
	} else if rsp.Duration > r.maxDuration {
		r.maxDuration = rsp.Duration
	}
	r.sumDuration += rsp.Duration
	// store one full event per logAndReset cycle
	if r.event == nil {
		ev := rsp.Metadata.(htevent.Event)
		r.event = &ev
	}
}

// log the current stats and reset them all to zero.
// thread safe.
func (r *responseStats) logAndReset() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.log()
	r.reset()
}

// log the current statistics to logrus.
// NOT thread safe.
func (r *responseStats) log() {
	var avg time.Duration
	if r.count != 0 {
		avg = r.sumDuration / time.Duration(r.count)
	} else {
		avg = 0
	}
	logrus.WithFields(logrus.Fields{
		"count":            r.count,
		"lifetime_count":   r.totalCount + r.count,
		"slowest":          r.maxDuration,
		"fastest":          r.minDuration,
		"avg_duration":     avg,
		"count_per_status": r.statusCodes,
		"response_bodies":  r.bodies,
		"errors":           r.errors,
	}).Info("Summary of sent events")
	if r.event != nil {
		fields := r.event.Data
		fields["event_timestamp"] = r.event.Timestamp
		logrus.WithFields(fields).Info("Sample parsed event")
	}
}

// log the total count on its own
func (r *responseStats) logFinal() {
	r.totalCount += r.count
	for code, count := range r.statusCodes {
		r.totalStatusCodes[code] += count
	}
	logrus.WithFields(logrus.Fields{
		"total attempted sends":               r.totalCount,
		"number sent by response status code": r.totalStatusCodes,
	}).Info("Total number of events sent")
}

// reset the counters to zero.
// NOT thread safe
func (r *responseStats) reset() {
	r.totalCount += r.count
	for code, count := range r.statusCodes {
		r.totalStatusCodes[code] += count
	}
	r.count = 0
	r.statusCodes = make(map[int]int)
	r.bodies = make(map[string]int)
	r.errors = make(map[string]int)
	r.maxDuration = 0
	r.sumDuration = 0
	r.minDuration = 0
	r.event = nil
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

func verifyWriteKeyConfig(userAgent string, c *WriteKeyConfig) error {
	url := fmt.Sprintf("%s/1/team_slug", c.ApiUrl)
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Add("X-Honeycomb-Team", c.WriteKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Couldn't verify write key with Honeycomb server (%q): %s", c.ApiUrl, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		// TODO: Ensure body is valid UTF-8?
		// TODO: Maybe use "%s" instead of "%q" if message doesn't contain any special characters.
		return fmt.Errorf("Couldn't verify write key with Honeycomb server (%q): HTTP %d: %q",
			c.ApiUrl, resp.StatusCode, string(body));
	}
	return nil
}
