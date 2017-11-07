// Package regex consumes logs given user-defined regex format for lines

/* Example usage (from honeytail root)
  		go install . && \
 			honeytail -p regex -k $HONEYTAIL_WRITEKEY \
			-f /private/var/system.log \
			--dataset 'MY_TEST_DATASET' \
			--backfill \
			--regex.line_regex="(?P<time>\d{2}:\d{2}:\d{2})" \
			--regex.timefield="time" \
			--regex.time_format="%H:%M:%S"
*/

package regex

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/httime"
	"github.com/honeycombio/honeytail/parsers"
)

type Options struct {
	LineRegex       string `long:"line_regex" description:"a regular expression with named capture groups representing the fields you want parsed"`
	TimeFieldName   string `long:"timefield" description:"Name of the field that contains a timestamp"`
	TimeFieldFormat string `long:"time_format" description:"Timestamp format to use (strftime and Golang time.Parse supported)"`

	NumParsers int `hidden:"true" description:"number of regex parsers to spin up"`
}

type Parser struct {
	conf       Options
	lineParser LineParser
}

func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)

	if p.conf.LineRegex != "" {
		var err error
		lineRegex, err := regexp.Compile(p.conf.LineRegex)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"LineRegex": p.conf.LineRegex,
			}).Debug("Could not compile line regex")
			return err
		}
		p.lineParser = &RegexLineParser{lineRegex}
	}

	return nil
}

type LineParser interface {
	ParseLine(line string) (map[string]interface{}, error)
}

type RegexLineParser struct {
	lineRegex *regexp.Regexp
}

func (p *RegexLineParser) ParseLine(line string) (map[string]interface{}, error) {
	parsed := make(map[string]interface{})
	if p.lineRegex == nil {
		logrus.Debug("RegexLineParser", p, "has no lineRegex")
		return parsed, errors.New("RegexLineParser instance has no lineRegex")
	}
	match := p.lineRegex.FindAllStringSubmatch(line, -1)
	if match == nil || len(match) == 0 {
		logrus.WithFields(logrus.Fields{
			"line": line,
		}).Debug("No matches for regex log line")
		return parsed, nil
	}

	// Map capture groups
	var firstMatch []string = match[0] // We only care about the first full lineRegex match
	for i, name := range p.lineRegex.SubexpNames() {
		if i != 0 && i < len(firstMatch) {
			parsed[name] = firstMatch[i]
		}
	}
	logrus.WithFields(logrus.Fields{
		"parsed": parsed,
		"line":   line,
	}).Debug("Regex parsing log line")

	return parsed, nil
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event, prefixRegex *parsers.ExtRegexp) {
	// parse lines one by one
	wg := sync.WaitGroup{}
	for i := 0; i < p.conf.NumParsers; i++ {
		wg.Add(1)
		go func() {
			for line := range lines {
				logrus.WithFields(logrus.Fields{
					"line": line,
				}).Debug("Attempting to process regex log line")

				// take care of any headers on the line
				var prefixFields map[string]string
				if prefixRegex != nil {
					var prefix string
					prefix, prefixFields = prefixRegex.FindStringSubmatchMap(line)
					line = strings.TrimPrefix(line, prefix)
				}

				parsedLine, err := p.lineParser.ParseLine(line)
				if err != nil {
					continue
				}

				// merge the prefix fields and the parsed line contents
				for k, v := range prefixFields {
					parsedLine[k] = v
				}

				if len(parsedLine) == 0 {
					logrus.WithFields(logrus.Fields{
						"line": line,
					}).Debug("Skipping line; no capture groups found")
					continue
				}

				// look for the timestamp in any of the prefix fields or regular content
				timestamp := httime.GetTimestamp(parsedLine, p.conf.TimeFieldName, p.conf.TimeFieldFormat)

				// send an event to Transmission
				e := event.Event{
					Timestamp: timestamp,
					Data:      parsedLine,
				}
				send <- e
			}
			wg.Done()
		}()
	}
	wg.Wait()
	logrus.Debug("lines channel is closed, ending regex processor")
}

type NoopLineParser struct {
	incomingLine string
	outgoingMap  map[string]interface{}
}

func (n *NoopLineParser) ParseLine(line string) (map[string]interface{}, error) {
	n.incomingLine = line
	return n.outgoingMap, nil
}
