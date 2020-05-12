package htjson

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/honeycombio/honeytail/event"
)

type testLineMap struct {
	input    string
	expected map[string]interface{}
}

var tlms = []testLineMap{
	{ // strings, floats, and ints
		input: `{"mystr": "myval", "myint": 3, "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"mystr":   "myval",
			"myint":   float64(3),
			"myfloat": 4.234,
		},
	},
	{ // time
		input: `{"time": "2014-03-10 19:57:38.123456789 -0800 PST", "myint": 3, "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"time":    "2014-03-10 19:57:38.123456789 -0800 PST",
			"myint":   float64(3),
			"myfloat": 4.234,
		},
	},
	{ // non-flat json object
		input: `{"array": [3, 4, 6], "obj": {"subkey":"subval"}, "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"array":   []interface{}{float64(3), float64(4), float64(6)},
			"obj":     map[string]interface{}{"subkey": "subval"},
			"myfloat": 4.234,
		},
	},
}

func TestParseLine(t *testing.T) {
	jlp := JSONLineParser{}
	for _, tlm := range tlms {
		resp, err := jlp.ParseLine(tlm.input)
		if err != nil {
			t.Error("jlp.ParseLine unexpectedly returned error ", err)
		}
		if !reflect.DeepEqual(resp, tlm.expected) {
			t.Errorf("response %+v didn't match expected %+v", resp, tlm.expected)
		}
	}
}

func TestProcessLinesHandlesMultilineJSONInput(t *testing.T) {
	// test data: has 3 json objects across multiple lines
	input := `
	"mid-object-beginning": 0}
	{
		"first": 1,
		"another_key": 1
	}
	{ "second": 2}
	{

		"third": 3

	}
	{
		"fourth, but incomplete": 0
`
	p := Parser{}
	if err := p.Init(&Options{}); err != nil {
		t.Errorf("Failed to init parser: %e", err)
	}

	inputLines := make(chan string)
	outputEvents := make(chan event.Event, 4) // 4 so there's room for an additional event if our test creates more than expected
	defer close(inputLines)

	go p.ProcessLines(inputLines, outputEvents, nil)

	for _, line := range strings.Split(input, "\n") {
		inputLines <- line
	}

	// TODO I'm sure there's a more correct way to do this, but it escapes me at
	// the moment
	time.Sleep(3 * time.Second)

	expected := 3
	found := len(outputEvents)
	if found != expected {
		t.Errorf("Expected %d outputs, got %d", expected, found)
	}
}
