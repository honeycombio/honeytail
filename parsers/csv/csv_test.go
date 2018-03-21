package csv

import (
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/parsers"
)

const (
	commonLogFormatTimeLayout = "02/Jan/2006:15:04:05 -0700"
	iso8601TimeLayout         = "2006-01-02T15:04:05-07:00"
)

// Test Init(...) success/fail

type testInitMap struct {
	options      *Options
	expectedPass bool
}

var testInitCases = []testInitMap{
	{
		expectedPass: true,
		options: &Options{
			NumParsers:      5,
			TimeFieldName:   "local_time",
			TimeFieldFormat: "%d/%b/%Y:%H:%M:%S %z",
			Fields:          "one, two, three",
		},
	},
	{
		expectedPass: false,
		options: &Options{
			NumParsers:      5,
			TimeFieldName:   "local_time",
			TimeFieldFormat: "%d/%b/%Y:%H:%M:%S %z",
			Fields:          "", // No fields specified should cause a failure
		},
	},
}

func TestInit(t *testing.T) {
	for _, testCase := range testInitCases {
		p := &Parser{}
		err := p.Init(testCase.options)
		if (err == nil) != testCase.expectedPass {
			if err == nil {
				t.Error("Parser Init(...) passed; expected it to fail.")
			} else {
				t.Error("Parser Init(...) failed; expected it to pass. Error:", err)
			}
		} else {
			t.Logf("Init pass status is %t as expected", (err == nil))
		}
	}
}

type testLineMap struct {
	fields   string
	input    string
	expected map[string]interface{}
	err      bool
}

var tlms = []testLineMap{
	{
		fields: "one,two,three",
		input:  "exx, why, zee",
		expected: map[string]interface{}{
			"one":   "exx",
			"two":   " why",
			"three": " zee",
		},
	},
	{
		// No data should return of error (field num mismatch)
		fields:   "one,two,three",
		input:    "",
		expected: nil,
		err:      true,
	},
	{
		// Too few fields should return an error
		fields:   "one,two,three",
		input:    "foo,bar",
		expected: nil,
		err:      true,
	},
	{
		// Too many fields should return an error
		fields:   "one, two",
		input:    "foo,bar,xyz",
		expected: nil,
		err:      true,
	},
	{
		// Test that int and float are converted successfully
		fields: "one,two,three",
		input:  "1,2.4,xyz",
		expected: map[string]interface{}{
			"one":   1,
			"two":   2.4,
			"three": "xyz",
		},
	},
}

func TestParseLine(t *testing.T) {
	for _, tlm := range tlms {
		p := &Parser{}
		err := p.Init(&Options{
			Fields: tlm.fields,
		})
		assert.NoError(t, err, "could not instantiate parser with fields: %s", tlm.fields)
		resp, err := p.lineParser.ParseLine(tlm.input)
		t.Logf("%+v", resp)
		if tlm.err {
			assert.Error(t, err, "p.ParseLine did not return error as expected")
		} else {
			assert.NoError(t, err, "p.ParseLine unexpectedly returned error %v", err)
		}
		if !reflect.DeepEqual(resp, tlm.expected) {
			t.Errorf("response %+v didn't match expected %+v", resp, tlm.expected)
		}
	}
}

func TestParseLineTrimWhitespace(t *testing.T) {
	tlm := testLineMap{
		fields: "one, two, three",
		input:  "exx, why, zee",
		expected: map[string]interface{}{
			"one":   "exx",
			"two":   "why",
			"three": "zee",
		},
	}

	p := &Parser{}
	err := p.Init(&Options{
		Fields:           tlm.fields,
		TrimLeadingSpace: true,
	})
	assert.NoError(t, err, "could not instantiate parser with fields: %s", tlm.fields)
	resp, err := p.lineParser.ParseLine(tlm.input)
	t.Logf("%+v", resp)
	if tlm.err {
		assert.Error(t, err, "p.ParseLine did not return error as expected")
	} else {
		assert.NoError(t, err, "p.ParseLine unexpectedly returned error %v", err)
	}
	if !reflect.DeepEqual(resp, tlm.expected) {
		t.Errorf("response %+v didn't match expected %+v", resp, tlm.expected)
	}
}

type testLineMaps struct {
	line        string
	trimmedLine string
	resp        map[string]interface{}
	typedResp   map[string]interface{}
	ev          event.Event
}

// Test event emitted from ProcessLines
func TestProcessLines(t *testing.T) {
	t1, _ := time.ParseInLocation(commonLogFormatTimeLayout, "08/Oct/2015:00:26:26 -0000", time.UTC)
	preReg := &parsers.ExtRegexp{regexp.MustCompile("^(?P<pre_hostname>[a-zA-Z-.]+): ")}
	tlm := []testLineMaps{
		{
			line: "somehost: 08/Oct/2015:00:26:26 +0000,123,xyz,:::",
			ev: event.Event{
				Timestamp: t1,
				Data: map[string]interface{}{
					"pre_hostname": "somehost",
					"one":          123,
					"two":          "xyz",
					"three":        ":::",
				},
			},
		},
	}
	p := &Parser{}
	err := p.Init(&Options{
		NumParsers:      5,
		TimeFieldName:   "local_time",
		TimeFieldFormat: "%d/%b/%Y:%H:%M:%S %z",
		Fields:          "local_time,one,two,three",
	})
	assert.NoError(t, err, "Couldn't instantiate Parser")

	lines := make(chan string)
	send := make(chan event.Event)
	go func() {
		for _, pair := range tlm {
			lines <- pair.line
		}
		close(lines)
	}()
	go p.ProcessLines(lines, send, preReg)
	for _, pair := range tlm {
		resp := <-send
		if !reflect.DeepEqual(resp, pair.ev) {
			t.Fatalf("line resp didn't match up for %s. Expected: %+v, actual: %+v",
				pair.line, pair.ev, resp)
		}
	}
}
