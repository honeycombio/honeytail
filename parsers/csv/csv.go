package csv

import (
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/httime"
	"github.com/honeycombio/honeytail/parsers"
)

// Options defines the options relevant to the CSV parser
type Options struct {
	Fields           string `long:"fields" description:"Comma separated list of CSV fields, in order."`
	TimeFieldName    string `long:"timefield" description:"Name of the field that contains a timestamp"`
	TimeFieldFormat  string `long:"time_format" description:"Timestamp format to use (strftime and Golang time.Parse supported)"`
	NumParsers       int    `hidden:"true" description:"number of csv parsers to spin up"`
	TrimLeadingSpace bool   `bool:"trim_leading_space" description:"trim leading whitespace in CSV fields and values" default:"false"`
}

// Parser implements the Parser interface
type Parser struct {
	conf       Options
	lineParser parsers.LineParser
}

// Init constructs our parser from the provided options
func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)
	if p.conf.Fields == "" {
		return errors.New("must provide at least 1 field name when parsing CSV lines")
	}
	lineParser, err := NewCSVLineParser(p.conf.Fields, p.conf.TrimLeadingSpace)
	if err != nil {
		return err
	}
	p.lineParser = lineParser
	return nil
}

type CSVLineParser struct {
	fields           []string
	numFields        int
	trimLeadingSpace bool
}

// NewCSVLineParser factory
func NewCSVLineParser(fieldsString string, trimLeadingSpace bool) (*CSVLineParser, error) {
	// Is building a reader for every single line a good idea?
	// Potential for future optimization here
	reader := strings.NewReader(fieldsString)
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = trimLeadingSpace

	fields, err := csvReader.Read()
	if err != nil {
		logrus.WithError(err).WithField("fields", fieldsString).
			Error("unable to parse list of fields")
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"fields": fields,
	}).Debug("generated CSV fields")
	return &CSVLineParser{
		fields:           fields,
		numFields:        len(fields),
		trimLeadingSpace: trimLeadingSpace}, nil
}

func (p *CSVLineParser) ParseLine(line string) (map[string]interface{}, error) {
	csvReader := csv.NewReader(strings.NewReader(line))
	csvReader.FieldsPerRecord = p.numFields
	csvReader.TrimLeadingSpace = p.trimLeadingSpace
	data := make(map[string]interface{})
	values, err := csvReader.Read()
	if err != nil {
		logrus.WithError(err).WithField("line", line).
			Error("failed to parse line")
		return nil, err
	}

	for i := 0; i < p.numFields; i++ {
		if val, err := strconv.Atoi(values[i]); err == nil {
			data[p.fields[i]] = val
		} else if val, err := strconv.ParseFloat(values[i], 64); err == nil {
			data[p.fields[i]] = val
		} else {
			data[p.fields[i]] = values[i]
		}
	}

	return data, nil
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
				}).Debug("attempting to process csv line")

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
	logrus.Debug("lines channel is closed, ending csv processor")
}
