package struct_extractor

import (
    "encoding/json"
    "testing"
	"reflect"
	"fmt"
)

func parse(serializedJson string) interface{} {
	var j interface{}
	err := json.Unmarshal([]byte(serializedJson), &j)
	if err != nil {
		panic(fmt.Sprintf("JSON didn't parse: %q: %s", serializedJson, err))
	}
	return j
}

type stuff struct {
	name string
	main *span
	alternates []*span
}

type span struct {
	start int
	end int
}

func extractStuff(v *Value) *stuff {
	m := v.Map()

	name := m.Pop("name").String()
	main := extractSpan(m.Pop("main"))
	var alternates []*span
	for _, alternateV := range m.Pop("alternates").List().All() {
		fmt.Printf("alternateV: %#v", alternateV)
		alternates = append(alternates, extractSpan(alternateV))
	}

	m.Done()
	return &stuff{name, main, alternates}
}

func extractSpan(v *Value) *span {
	m := v.Map()

	start := m.Pop("start").Int32()

	end := -1
	m.PopMaybeAnd("end", func(v *Value) {
		end = v.Int32()
		if end < start {
			m.Fail("\"end\" must not be less than \"start\"; got start=%d and end=%d", start, end)
		}
	})

	m.Done()
	return &span{start, end}
}

func TestBasic(t *testing.T) {
	expectError := func(data string, expected string) {
		j := parse(data)
		err := Run(j, func(v *Value) {
			stuff := extractStuff(v)
			t.Fatalf("unexpected successful extraction: %#v", stuff)
		})
		if err == nil {
			t.Fatalf("expecting %q got no error", expected)
		}
		if err.Error() != expected {
			t.Fatalf("expecting %q got %q", expected, err.Error())
		}
	}

	expectOk := func(data string, expected *stuff) {
		j := parse(data)
		var stuff *stuff
		err := Run(j, func(v *Value) {
			stuff = extractStuff(v)
		})
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if !reflect.DeepEqual(expected, stuff) {
			t.Fatalf("expected %#v got %#v", expected, stuff)
		}
	}

	expectOk(
		`{
			"name": "blah",
			"main": { "start": 12 },
			"alternates": [{"start": 18, "end": 18}]
		}`,
		&stuff{
			"blah",
			&span{12, -1},
			[]*span{{18, 18}},
		},
	)

	expectError(
		`12`,
		`expecting object, got number`,
	)
	expectError(
		`[{}]`,
		`expecting object, got list`,
	)
	expectError(
		`{
			"name": "blah",
			"main": {"start": 14, "end": true},
			"alternates": []
		}`,
		`"main": "end": expecting integer, got boolean`,
	)
	expectError(
		`{
			"name": "blah",
			"main": {"start": 14, "end": 17},
			"alternates": [{"start": 14, "end": 13}]
		}`,
		`"alternates": index 0: "end" must not be less than "start"; got start=14 and end=13`,
	)
}
