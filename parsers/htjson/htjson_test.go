package htjson

import (
	"reflect"
	"testing"
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

type testFlattenMap struct {
	input     map[string]interface{}
	expected  map[string]interface{}
	depth     int
	delimiter string
}

var tfms = []testFlattenMap{
	testFlattenMap{
		// depth is nonzero but there's nothing to do
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": 3,
		},
		expected: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": 3,
		},
		depth:     3,
		delimiter: ".",
	},
	testFlattenMap{
		// there's a nested map but depth is 0, so we should get the map back
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": 4,
				"e": 5,
			},
		},
		expected: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": 4,
				"e": 5,
			},
		},
		depth:     0,
		delimiter: ".",
	},
	testFlattenMap{
		// there's a nested map and depth is 1, so expect a flat map
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": 4,
				"e": 5,
			},
		},
		expected: map[string]interface{}{
			"a":   1,
			"b":   2,
			"c.d": 4,
			"c.e": 5,
		},
		depth:     1,
		delimiter: ".",
	},
	testFlattenMap{
		// there's are multiple levels of nesting, but depth is 1,
		// so we should only go one level deep
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": map[string]interface{}{"f": 6},
				"e": 5,
			},
		},
		expected: map[string]interface{}{
			"a":   1,
			"b":   2,
			"c.d": map[string]interface{}{"f": 6},
			"c.e": 5,
		},
		depth:     1,
		delimiter: ".",
	},
	testFlattenMap{
		// test mutliple nested maps with varying levels of depth
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": map[string]interface{}{"f": 6},
				"e": 5,
			},
			"x": map[string]interface{}{
				"y": map[string]interface{}{
					"z1": 100,
					"z2": 200,
				},
			},
		},
		expected: map[string]interface{}{
			"a":      1,
			"b":      2,
			"c.d.f":  6,
			"c.e":    5,
			"x.y.z1": 100,
			"x.y.z2": 200,
		},
		depth:     10,
		delimiter: ".",
	},
	testFlattenMap{
		// test alternate delimiters
		input: map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": map[string]interface{}{
				"d": 4,
				"e": 5,
			},
		},
		expected: map[string]interface{}{
			"a":     1,
			"b":     2,
			"c---d": 4,
			"c---e": 5,
		},
		depth:     1,
		delimiter: "---",
	},
}

func TestFlatten(t *testing.T) {
	for _, tfm := range tfms {
		flatten(tfm.input, tfm.delimiter, tfm.depth)
		if !reflect.DeepEqual(tfm.input, tfm.expected) {
			t.Errorf("flattened input %+v didn't match expected %+v", tfm.input, tfm.expected)
		}
	}
}
