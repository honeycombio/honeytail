package reporting

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/dynsampler-go"
	"github.com/honeycombio/libhoney-go"

	"github.com/honeycombio/honeytail/event"
)

const (
	contextKeyBuilder = "builder"
	contextKeySampler = "sampler"
	reportingSuffix   = "-reports"
)

func NewContext(ctx context.Context) context.Context {
	builder := libhoney.NewBuilder()
	builder.Dataset += reportingSuffix
	if hostname, err := os.Hostname(); err == nil {
		builder.AddField("hostname", hostname)
	}

	sampler := &dynsampler.PerKeyThroughput{
		ClearFrequencySec:      1,
		PerKeyThroughputPerSec: 10,
	}
	ctx = context.WithValue(ctx, contextKeyBuilder, builder)
	if err := sampler.Start(); err != nil {
		logrus.WithField("error", err).Error("Unexpected error initializing dynamic sampler")
		return ctx
	}
	return context.WithValue(ctx, contextKeySampler, sampler)
}

func Options(ctx context.Context, options interface{}) {
	if ev := getEvent(ctx, "options"); ev != nil {
		ev.AddField("config_json", options)
		ev.Send()
	}
}

func TailState(ctx context.Context, file string, inode uint64, offset int64, size int64) {
	if ev := getEvent(ctx, "tail snapshot"); ev != nil {
		ev.AddField("tail_filename", file)
		ev.AddField("tail_inode", inode)
		ev.AddField("tail_offset", offset)
		ev.AddField("tail_filesize", size)
		ev.AddField("pct_processed", float64(offset)/float64(size)*100)
		ev.Send()
	}
}

func ParseError(ctx context.Context, line string, err error) {
	logrus.WithFields(logrus.Fields{
		"line":  line,
		"error": err,
	}).Debugln("Skipped: log line failed to parse")

	if ev := getEvent(ctx, "parse error"); ev != nil {
		ev.AddField("log_line", line)
		ev.AddField("parse_error", err.Error())
		ev.Send()
	}
}

func Skip(ctx context.Context, line, skipReason string) {
	SkipWithFields(ctx, line, skipReason, nil)
}

func SkipWithFields(ctx context.Context, line, skipReason string, fields logrus.Fields) {
	logrus.WithFields(fields).WithField("line", line).Debugln("Skipped:", skipReason)

	if ev := getEvent(ctx, "skip"); ev != nil {
		ev.AddField("log_line", line)
		ev.AddField("skip_reason", skipReason)
		ev.Send()
	}
}

func SendError(ctx context.Context, sentEvent *event.Event, err error) {
	logrus.WithFields(logrus.Fields{
		"event": sentEvent,
		"error": err,
	}).Error("Unexpected error when sending to Honeycomb")

	if ev := getEvent(ctx, "send error"); ev != nil {
		ev.AddField("event_timestamp", sentEvent.Timestamp)
		ev.AddField("event_timestamp_lag_sec", time.Since(sentEvent.Timestamp)/time.Second)
		ev.AddField("event_data", sentEvent.Data)
		ev.AddField("honeycomb_error", err.Error())
		ev.Send()
	}
}

func Response(ctx context.Context, rsp *libhoney.Response, willRetry bool) {
	logrus.WithFields(logrus.Fields{
		"status_code": rsp.StatusCode,
		"body":        strings.TrimSpace(string(rsp.Body)),
		"duration":    rsp.Duration,
		"error":       rsp.Err,
	}).Debug("Server response")

	if ev := getEvent(ctx, "response"); ev != nil {
		defer func() {
			// If we've called libhoney.Close(), response handling will still
			// happen - and we should be fine with just tossing away telemetry
			// as a result
			recover()
		}()

		sentEvent := rsp.Metadata.(event.Event)
		ev.AddField("event_timestamp", sentEvent.Timestamp)
		ev.AddField("event_timestamp_lag_sec", time.Since(sentEvent.Timestamp)/time.Second)
		ev.AddField("event_data", sentEvent.Data)
		ev.AddField("response_status_code", rsp.StatusCode)
		ev.AddField("response_latency_ms", rsp.Duration/time.Millisecond)
		if rsp.StatusCode > 400 {
			ev.AddField("honeycomb_error", rsp.Err.Error())
		}
		ev.Send()
	}
}

func getEvent(ctx context.Context, key string) *libhoney.Event {
	builder, ok := ctx.Value(contextKeyBuilder).(*libhoney.Builder)
	if !ok { // will bail if not configured to report
		return nil
	}

	ev := builder.NewEvent()
	if sampler, ok := ctx.Value(contextKeySampler).(dynsampler.Sampler); ok {
		ev.SampleRate = uint(sampler.GetSampleRate(key))
	}

	return ev
}
