// Package nginx consumes nginx logs
package nginx

import (
	"os"
	"strconv"
	"strings"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/gonx"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_wrapper "github.com/honeycombio/honeytail/v2/parser/wrapper"
)

const (
	commonLogFormatTimeLayout = "02/Jan/2006:15:04:05 -0700"
	iso8601TimeLayout         = "2006-01-02T15:04:05-07:00"
)

func Configure(v *sx.Value) htparser.BuildFunc {
	var configFile string
	var logFormatName string

	v.Map(func(m sx.Map) {
		configFile = m.Pop("config_file").String()
		logFormatName = m.Pop("log_format_name").String()
	})

	setupLineParser := func() (htparser_wrapper.LineParserFactory, error) {
		configFileH, err := os.Open(configFile)
		if err != nil {
			return nil, fmt.Errorf("couldn't open \"config_file\" %q: %s", configFile, err)
		}
		defer configFileH.Close()

		gonxParser, err := gonx.NewNginxParser(configFileH, logFormatName)
		if err != nil {
			return nil, fmt.Errorf("unable to determine log format from Nginx config file %q and log format name %q: %s",
				configFile, logFormatName, err)
		}

		lineParser := func(line string, sendEvent htparser.SendEvent) {
			logrus.WithFields(logrus.Fields{
				"line": line,
			}).Debug("Attempting to process nginx log line")

			gonxEvent, err := gonxParser.ParseString(line)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"logline": line,
				}).Debug("gonx.ParseString failed on log line", err.Error())
				return
			}
			fields := typeifyParsedLine(gonxEvent.Fields)
			sendEvent(getTimestamp(fields), fields)
		}

		// We don't have any thread-local setup, so just return the same instance.
		lineParserFactory := func() htparser_wrapper.LineParser { return lineParser }

		return lineParserFactory, nil
	}

	return htparser_wrapper.LineParserWrapper(setupLineParser)
}

// typeifyParsedLine attempts to cast numbers in the event to floats or ints
func typeifyParsedLine(pl map[string]string) map[string]interface{} {
	// try to convert numbers, if possible
	msi := make(map[string]interface{}, len(pl))
	for k, v := range pl {
		switch {
		case strings.Contains(v, "."):
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				msi[k] = f
				continue
			}
		case v == "-":
			// no value, don't set a "-" string
			continue
		default:
			i, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				msi[k] = i
				continue
			}
		}
		msi[k] = v
	}
	return msi
}

// tries to extract a timestamp from the log line
// Returns the zero time value `time.Time{}` if extraction fails.
func getTimestamp(evMap map[string]interface{}) time.Time {
	var err error
	var timestamp time.Time = time.Time{}

	defer delete(evMap, "time_local")
	defer delete(evMap, "time_iso8601")

	if val, ok := evMap["time_local"]; ok {
		rawTime, found := val.(string)
		if !found {
			// unable to parse string. log and return Now()
			logrus.WithFields(logrus.Fields{
				"expected_time": val,
			}).Debug("unable to coerce expected time to string")
			return timestamp
		}
		timestamp, err = time.Parse(commonLogFormatTimeLayout, rawTime)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw_time": rawTime,
			}).Debug("unable to parse time", err.Error())
		}
	} else if val, ok := evMap["time_iso8601"]; ok {
		rawTime, found := val.(string)
		if !found {
			// unable to parse string. log and return Now()
			logrus.WithFields(logrus.Fields{
				"expected_time": val,
			}).Debug("unable to coerce expected time to string")
			return timestamp
		}
		timestamp, err = time.Parse(iso8601TimeLayout, rawTime)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"raw_time": rawTime,
			}).Debug("unable to parse time", err.Error())
		}
	}

	return timestamp
}
