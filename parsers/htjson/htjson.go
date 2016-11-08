// Package htjson (honeytail-json, renamed to not conflict with the json module)
// parses logs that are one json blob per line.
package htjson

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/honeycombio/honeytail/event"
)

var possibleTimeFieldNames = []string{
	"time", "Time",
	"timestamp", "Timestamp", "TimeStamp",
	"date", "Date",
	"datetime", "Datetime", "DateTime",
}

type Options struct {
	TimeFieldName string `long:"timefield" description:"Name of the field that contains a timestamp"`
	Format        string `long:"format" description:"Format of the timestamp found in timefield (supports strftime and Golang time formats)"`
}

type parserStats struct {
	LinesEncountered int
	LinesErrored     int
	EventsParsed     int
	lock             sync.Mutex
}

type Parser struct {
	conf       Options
	lineParser LineParser
	nower      Nower
	stats      parserStats
}

type Nower interface {
	Now() time.Time
}

type RealNower struct{}

func (r *RealNower) Now() time.Time {
	return time.Now().UTC()
}

func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)

	p.nower = &RealNower{}
	p.lineParser = &JSONLineParser{}
	return nil
}

type LineParser interface {
	ParseLine(line string) (map[string]interface{}, error)
}

type JSONLineParser struct {
}

// LineParser will do a complete JSON decode of the line,
// but then re-encode any value that's not a string as JSON and return
// it as a string. We don't want nested objects, but it seems silly to
// balk instead of just pushing json as the value into retriever.
func (j *JSONLineParser) ParseLine(line string) (map[string]interface{}, error) {
	parsed := make(map[string]interface{})
	err := json.Unmarshal([]byte(line), &parsed)
	if err != nil {
		return nil, err
	}
	processed := make(map[string]interface{})
	for k, v := range parsed {
		switch typedVal := v.(type) {
		case bool, string, float64:
			processed[k] = typedVal
		default:
			rejsoned, _ := json.Marshal(v)
			processed[k] = string(rejsoned)

		}
	}
	return processed, err
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event) {
	for line := range lines {
		p.stats.LinesEncountered++
		logrus.WithFields(logrus.Fields{
			"line": line,
		}).Debug("Attempting to process json log line")
		parsedLine, err := p.lineParser.ParseLine(line)
		if err != nil {
			// count and skip lines that won't parse
			p.stats.LinesErrored++
			logrus.WithFields(logrus.Fields{
				"line": line,
			}).Debug("skipping line; failed to parse.")
			continue
		}
		timestamp := p.getTimestamp(parsedLine)
		p.stats.EventsParsed++

		// send an event to Transmission
		e := event.Event{
			Timestamp: timestamp,
			Data:      parsedLine,
		}
		send <- e
	}
	logrus.Debug("lines channel is closed, ending json processor")
}

// getTimestamp looks through the event map for something that looks
// like a timestamp. It will guess at the key name or use
// the one from Config if it is not ""
// if unable to parse it will return the current time
// it is highly recommended that you populate the Config with time format
// sample from logrus: "time":"2014-03-10 19:57:38.562264131 -0400 EDT"
func (p *Parser) getTimestamp(m map[string]interface{}) time.Time {
	var ts time.Time
	if p.conf.TimeFieldName != "" {
		// remove the timestamp from the body when we stuff it in the header
		defer delete(m, p.conf.TimeFieldName)
		if t, found := m[p.conf.TimeFieldName]; found {
			ts = p.tryTimeFormats(t.(string))
			if ts.IsZero() {
				// we found the time field but failed to parse it
				ts = p.nower.Now()
			}
		} else {
			// we were told to look for specific time field and failed to find it
			ts = p.nower.Now()
		}
		// we were told to look for a specific field;
		// let's return what we found instead of continuing to look.
		return ts
	}
	// go through all the possible fields that might have a timestamp
	// for the first one we find, if it's a string field, try and parse it
	// if we succeed, stop looking. Otherwise keep trying
	for _, timeField := range possibleTimeFieldNames {
		if t, found := m[timeField]; found {
			timeStr, found := t.(string)
			if found {
				defer delete(m, timeField)
				ts = p.tryTimeFormats(timeStr)
				if !ts.IsZero() {
					break
				}
			}
		}
	}
	if ts.IsZero() {
		ts = p.nower.Now()
	}
	return ts
}

func (p *Parser) LogStats() {
	logrus.WithFields(logrus.Fields{
		"lines_encountered": p.stats.LinesEncountered,
		"lines_errored":     p.stats.LinesErrored,
		"events_parsed":     p.stats.EventsParsed,
	}).Info("json parser stats")
	p.resetStats()
}

func (p *Parser) resetStats() {
	p.stats.lock.Lock()
	defer p.stats.lock.Unlock()
	p.stats.LinesEncountered = 0
	p.stats.LinesErrored = 0
	p.stats.EventsParsed = 0
}

func (p *Parser) tryTimeFormats(t string) time.Time {
	// golang can't parse times with decimal fractional seconds marked by a comma
	// hack it by just replacing all commas with periods and hope it works out.
	// https://github.com/golang/go/issues/6189
	t = strings.Replace(t, ",", ".", -1)
	if p.conf.Format == UnixTimestampFmt {
		if unix, err := strconv.ParseInt(t, 0, 64); err == nil {
			return time.Unix(unix, 0)
		}
	}
	if p.conf.Format != "" {
		format := strings.Replace(p.conf.Format, ",", ".", -1)
		if strings.Contains(format, StrftimeChar) {
			if ts, err := time.Parse(convertTimeFormat(format), t); err == nil {
				return ts
			}
		}

		// Still try Go style, just in case
		if ts, err := time.Parse(format, t); err == nil {
			return ts
		}
	}

	var ts time.Time
	if tOther, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", t); err == nil {
		ts = tOther
	} else if tOther, err := time.Parse(time.RFC3339Nano, t); err == nil {
		ts = tOther
	} else if tOther, err := time.Parse(time.RubyDate, t); err == nil {
		ts = tOther
	} else if tOther, err := time.Parse(time.UnixDate, t); err == nil {
		ts = tOther
	}
	return ts
}
