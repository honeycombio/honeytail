package syslog

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
			Mode:       "rfc3164",
			NumParsers: 5,
		},
	},
	{
		expectedPass: false,
		options: &Options{
			Mode: "foo", // unsupported syslog mode should cause a failure
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
	mode        string
	processList string
	input       string
	expected    map[string]interface{}
	err         bool
}

var tlms = []testLineMap{
	// these test cases taken from https://github.com/jeromer/syslogparser/
	{
		input: "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8",
		mode:  "rfc3164",
		expected: map[string]interface{}{
			"timestamp": time.Date(time.Now().Year(), 10, 11, 22, 14, 15, 0, time.UTC),
			"hostname":  "mymachine",
			"process":   "su",
			"content":   "'su root' failed for lonvick on /dev/pts/8",
			"priority":  34,
			"facility":  4,
			"severity":  2,
		},
	},
	{
		input: "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8",
		mode:  "rfc3164",
		// should parse, because su is in the list of processes
		processList: "su,sshd",
		expected: map[string]interface{}{
			"timestamp": time.Date(time.Now().Year(), 10, 11, 22, 14, 15, 0, time.UTC),
			"hostname":  "mymachine",
			"process":   "su",
			"content":   "'su root' failed for lonvick on /dev/pts/8",
			"priority":  34,
			"facility":  4,
			"severity":  2,
		},
	},
	{
		input: "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8",
		mode:  "rfc3164",
		// should not parse (but not return an error) since su is not in the list of processes
		processList: "sshd",
		expected:    nil,
	},
	{
		input: `<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry...`,
		mode:  "rfc5424",
		expected: map[string]interface{}{
			"version":         1,
			"timestamp":       time.Date(2003, 10, 11, 22, 14, 15, int(time.Millisecond*3), time.UTC),
			"structured_data": `[exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"]`,
			"proc_id":         "-",
			"hostname":        "mymachine.example.com",
			"severity":        5,
			"facility":        20,
			"priority":        165,
			"message":         "An application event log entry...",
			"msg_id":          "ID47",
			"process":         "evntslog",
		},
	},
}

func TestParseLine(t *testing.T) {
	for _, tlm := range tlms {
		p := &Parser{}
		err := p.Init(&Options{
			Mode:        tlm.mode,
			ProcessList: tlm.processList,
		})
		assert.NoError(t, err, "could not instantiate parser with mode: %s", tlm.mode)
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

type testLineMaps struct {
	line        string
	trimmedLine string
	resp        map[string]interface{}
	typedResp   map[string]interface{}
	ev          event.Event
}

// Test event emitted from ProcessLines
func TestProcessLines(t *testing.T) {
	preReg := &parsers.ExtRegexp{regexp.MustCompile("^(?P<pre_hostname>[a-zA-Z-.]+): ")}
	tlm := []testLineMaps{
		{
			line: "somehost: <34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8",
			ev: event.Event{
				Timestamp: time.Date(time.Now().Year(), 10, 11, 22, 14, 15, 0, time.UTC),
				Data: map[string]interface{}{
					"pre_hostname": "somehost",
					"timestamp":    time.Date(time.Now().Year(), 10, 11, 22, 14, 15, 0, time.UTC),
					"hostname":     "mymachine",
					"process":      "su",
					"content":      "'su root' failed for lonvick on /dev/pts/8",
					"priority":     34,
					"facility":     4,
					"severity":     2,
				},
			},
		},
	}
	p := &Parser{}
	err := p.Init(&Options{
		Mode:       "rfc3164",
		NumParsers: 5,
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
