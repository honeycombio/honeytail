package syslog

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/parsers"

	"github.com/jeromer/syslogparser"
	"github.com/jeromer/syslogparser/rfc3164"
	"github.com/jeromer/syslogparser/rfc5424"
)

// Options defines the options relevant to the syslog parser
type Options struct {
	Mode        string `long:"mode" description:"Syslog mode. Supported values are rfc3164 and rfc5424"`
	ProcessList string `long:"processes" description:"comma separated list of processes to filter for. example: 'sshd,sudo' - by default all are consumed"`
	NumParsers  int    `hidden:"true" description:"number of parsers to spin up"`
}

// Parser implements the Parser interface
type Parser struct {
	conf       Options
	lineParser parsers.LineParser
}

// Init constructs our parser from the provided options
func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)
	lineParser, err := NewSyslogLineParser(p.conf.Mode, p.conf.ProcessList)
	if err != nil {
		return err
	}
	p.lineParser = lineParser
	return nil
}

type SyslogLineParser struct {
	mode               string
	supportedProcesses map[string]struct{}
}

func normalizeLogFields(fields map[string]interface{}) {
	// The RFC3164 and RFC5424 parsers use different fields to refer to the
	// process - normalize to "process" for consistency and clarity
	// RFC3164
	if process, ok := fields["tag"].(string); ok {
		fields["process"] = process
		delete(fields, "tag")
	}
	// RFC5424
	if process, ok := fields["app_name"].(string); ok {
		fields["process"] = process
		delete(fields, "app_name")
	}

	// clean up whitespace in the message
	if message, ok := fields["message"].(string); ok {
		fields["message"] = strings.TrimSpace(message)
	}
}

// NewSyslogLineParser factory
func NewSyslogLineParser(mode string, processList string) (*SyslogLineParser, error) {
	var supportedProcesses map[string]struct{}
	// if a list of process
	if processList != "" {
		supportedProcesses = make(map[string]struct{})
		for _, process := range strings.Split(processList, ",") {
			supportedProcesses[strings.TrimSpace(process)] = struct{}{}
		}
	}
	if mode == "rfc3164" || mode == "rfc5424" {
		return &SyslogLineParser{
			mode:               mode,
			supportedProcesses: supportedProcesses,
		}, nil
	}

	return nil, fmt.Errorf("unsupported mode %s, see --help", mode)
}

func (p *SyslogLineParser) ParseLine(line string) (map[string]interface{}, error) {
	var parser syslogparser.LogParser
	if p.mode == "rfc3164" {
		parser = rfc3164.NewParser([]byte(line))
	} else if p.mode == "rfc5424" {
		parser = rfc5424.NewParser([]byte(line))
	}

	if err := parser.Parse(); err != nil {
		return nil, err
	}
	logFields := parser.Dump()
	normalizeLogFields(logFields)
	// if someone set --processes, this will not be nil
	if p.supportedProcesses != nil {
		if process, ok := logFields["process"].(string); ok {
			// if the process is not in the whitelist, skip it
			if _, match := p.supportedProcesses[process]; !match {
				return nil, nil
			}
		}
	}
	return logFields, nil
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event, prefixRegex *parsers.ExtRegexp) {
	// parse lines one by one
	wg := sync.WaitGroup{}
	numParsers := 1
	if p.conf.NumParsers > 0 {
		numParsers = p.conf.NumParsers
	}
	for i := 0; i < numParsers; i++ {
		wg.Add(1)
		go func() {
			for line := range lines {
				logrus.WithFields(logrus.Fields{
					"line": line,
				}).Debug("attempting to process line")

				// take care of any headers on the line
				var prefixFields map[string]string
				if prefixRegex != nil {
					var prefix string
					prefix, prefixFields = prefixRegex.FindStringSubmatchMap(line)
					line = strings.TrimPrefix(line, prefix)
				}

				parsedLine, err := p.lineParser.ParseLine(line)
				if parsedLine == nil || err != nil {
					continue
				}

				if len(parsedLine) == 0 {
					logrus.WithFields(logrus.Fields{
						"line": line,
					}).Info("skipping line, no values found")
					continue
				}

				// merge the prefix fields and the parsed line contents
				for k, v := range prefixFields {
					parsedLine[k] = v
				}

				// the timestamp should be in the log file if we're following either rfc 3164 or 5424
				var timestamp time.Time
				if t, ok := parsedLine["timestamp"].(time.Time); ok {
					timestamp = t
				}

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
	logrus.Debug("lines channel is closed, ending syslog processor")
}
