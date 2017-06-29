package struct_extractor

import (
	"fmt"
	"errors"
	"strings"
	"math"
)

// A library for validating dynamic list/map/string/int structures, e.g. the kind
// returned by JSON or YAML parsers.
//
// More flexible than the default validation provided by json.Unmarshal in a few
// ways: error message includes context on where the error occurred; unknown fields
// are reported by default (to catch misspellings, etc); can provide custom validation
// to check deeper properties.
//
// Let's say these are the types you want to parse from JSON/YAML:
//     type Color int
//     const (
//        Black = iota
//        White
//     )
//
//     type Point struct {
//        Color Color
//        X, Y int
//     }
// First, write some extraction functions:
//     import (
//         ...
//         sx ".../struct_extractor"
//     )
//
//     func ExtractColor(v *sx.Value) Color {
//        var s string = v.String()
//        switch s {
//        case "black": return Black
//        case "white": return White
//        default:
//           v.Fail("unknown color: %q", s)
//           panic("impossible")  // Unreachable, because Fail(...) panics.
//        }
//     }
//
//     func ExtractPoint(v *sx.Value) Point {
//        var p Point
//        v.Map(func(m Map) {
//            p.color = ExtractColor(m.Pop("color"))
//            p.x = m.Pop("x").Int32()
//
//            p.y = p.x  // defaults to the same as 'x'
//            m.PopMaybeAnd("y", func(v *sx.Value) { p.y = v.Int32() })
//        })
//        return p
//     }
// To run the extraction:
//     var err error
//     var v interface{}
//     err = json.Unmarshal(data, &v)  // Could also use a YAML library.
//     if err != nil {
//         return fmt.Errorf("Not valid JSON: %s\n", err)
//     }
//
//     // Must always wrap calls to extraction functions within sx.Run(...).
//     // Extraction functions use v.Fail(...), which panics.  sx.Run(...)
//     // handles those panics and converts them to regular error values.
//     var point Point
//     err = sx.Run(v, func(v *sx.Value) {
//         point = ExtractPoint(v)
//     })
//     if err != nil {
//         return err
//     }

// A dyamically typed value.  Use helpers like .String(), .List(), .Map(), and
// .Int32() to check if it's the shape you expect.
type Value struct {
	// Points to the value that this value is derived from.
	parent    *Value
	// If the parent value is a map: the field name that this value is under.
	fieldName *string
	// If the parent value is a list: the list index this value is under.
	listIndex int
	// The actual raw value.
	inner     interface{}
}

type Map struct {
	source *Value
	inner  map[string]interface{}
}

type List struct {
	source *Value
	inner  []interface{}
}

type wrappedError struct {
	root *Value
	err error
}

// All extraction work must be performed within a Run().  See the package
// documentation for an example.  Returns an error if any of the extraction
// work called Value.Fail(...).
func Run(value interface{}, parser func(*Value)) (err error) {
	err = nil

	root := Value{
		parent: nil,
		fieldName: nil,
		listIndex: -1,
		inner: value,
	}
	defer func() {
		r := recover()
		if r != nil {
			// If the panic value is ours, convert it to an error return.
			wrappedError, ok := r.(*wrappedError)
			if ok {
				if wrappedError.root != &root {
					// This should never happen.  But if it does, we can just add this
					// as a guard.
					panic(fmt.Sprintf("Different roots: %#v != %#v", wrappedError.root, &root))
				}
				err = wrappedError.err
				return
			}
			// If the panic value isn't ours, re-panic it.  Sadly, the new panic's
			// stack trace won't be identical to the original one -- this deferred
			// function will now be at the top -- but it's still pretty close.
			panic(r)
		}
	}()

	parser(&root)

	return
}

// Call this when something is wrong with 'v'.  For example:
//     s := v.String()
//     if (isValidEmail(s)) {
//         v.Fail("not a valid email address: %q", s)
//     }
func (v *Value) Fail(format string, args ...interface{}) interface{} {
	parts := []string{ fmt.Sprintf(format, args...) }

	// Walk up the Value.parent chain and append the context of where 'v' is
	// in the overall structure.
	curr := v
	for ; curr.parent != nil; {

		var part string
		if curr.fieldName != nil {
			part = fmt.Sprintf("%q", *curr.fieldName)
		} else {
			part = fmt.Sprintf("index %d", curr.listIndex)
		}
		parts = append(parts, part)

		curr = curr.parent
	}

	// Reverse the components, so that outer components appear first.
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	err := errors.New(strings.Join(parts, ": "))
	panic(&wrappedError{curr, err})
}

// A convenience; just calls the underlying Value's Fail(...).
func (m Map) Fail(format string, args ...interface{}) {
	m.source.Fail(format, args...)
}

// A conveneince; just calls the underlying Value's Fail(...).
func (l List) Fail(format string, args ...interface{}) {
	l.source.Fail(format, args...)
}

func (v *Value) intB(min int64, max int64) int64 {
	f, ok := v.inner.(float64)
	if !ok {
		v.Fail("expecting integer, got %s", DescribeValueType(v.inner))
	}
	if math.Trunc(f) != f {
		v.Fail("expecting integer, got non-integer number")
	}

	i := int64(f)
	if i < min || i > max {
		v.Fail("integer value out of range; expecting from %d to %d, got %d", min, max, i)
	}

	return i
}

// Expects the value to be a 32-bit signed integer.  If it's not, calls Fail(...).
func (v *Value) Int32() int32 {
	return int32(v.intB(math.MinInt32, math.MaxInt32))
}

// Expects the value to be a 32-bit signed integer with the give min/max bounds (inclusive).
// If it's not, calls Fail(...).
func (v *Value) Int32B(min int32, max int32) int32 {
	return int32(v.intB(int64(min), int64(max)))
}

// Expects the value to be a 32-bit unsigned integer.  If it's not, calls Fail(...).
func (v *Value) UInt32() uint32 {
	return uint32(v.intB(0, math.MaxUint32))
}

// Expects the value to be a 32-bit unsigned integer with the give min/max bounds (inclusive).
// If it's not, calls Fail(...).
func (v *Value) UInt32B(min uint32, max uint32) uint32 {
	return uint32(v.intB(int64(min), int64(max)))
}

// Expects the value to be a string.  If it's not, calls Fail(...).
func (v *Value) String() string {
	s, ok := v.inner.(string)
	if !ok {
		v.Fail("expecting string, got %s", DescribeValueType(v.inner))
	}
	return s
}

// If the value is a string, returns (s, true).  Otherwise, returns (..., false).
func (v *Value) TryString() (string, bool) {
	s, ok := v.inner.(string)
	if !ok {
		return "", false
	}
	return s, true
}

// Expects the value to be a map with a single key and value, then returns (key, value).
// If it's not, calls Fail(...).
func (v *Value) TaggedUnion() (string, *Value) {
	m := v.RawMap()
	kvs := m.PopAll()
	if len(kvs) != 1 {
		m.Fail("expecting a single entry, got %d.", len(kvs))
	}
	return kvs[0].Key, kvs[0].Value
}

// Returns the underlying untyped value.
func (v *Value) Any() interface{} {
	return v.inner
}

// Expects the value to be a map.  If it's not, calls Fail(...).
//
// Normally, you should use Map(...) instead, which takes care of calling Map.Done()
// for you automatically.
func (v *Value) RawMap() Map {
	m, ok := v.TryRawMap()
	if !ok {
		v.Fail("expecting object, got %s", DescribeValueType(v.inner))
	}
	return m
}

// If the value is a map, returns (map, true), otherwise returns (..., false).
//
// Normally, you should use Map(...) instead, which takes care of calling Map.Done()
// for you automatically.
func (v *Value) TryRawMap() (Map, bool) {
	m, ok := v.inner.(map[string]interface{})
	if !ok {
		return Map{}, false
	}
	return Map{
		source: v,
		inner: m,
	}, true
}

// If the value is not a map, calls Fail(...).  Otherwise, calls the given function
// to process the map.  When that function returns, calls Map.Done() to ensure
// that all fields were processed.
//
// If you want to ignore unknown fields instead of Fail'ing on them, call
// Map.IgnoreRest() after you're done processing the known fields.
func (v *Value) Map(f func(m Map)) {
	m := v.RawMap()
	f(m)
	m.Done()
}

// Make sure that all fields in the Map were Pop'ed.  If it's not, Fail(...) with
// the names of one of the keys that wasn't processed.  This is to ensure that
// unknown fields in the input are reported.
func (m Map) Done() {
	// Make sure all fields were popped.
	if len(m.inner) > 0 {
		// Get the minimum field name so that the error message is always the
		// same (and not based on the hash table's internal ordering).
		var min *string
		for fieldName := range m.inner {
			if min == nil {
				min = &fieldName
			} else if fieldName < *min {
				min = &fieldName
			}
		}
		m.Fail("unrecognized field: %q", *min)
	}
}

// Pop off a field for processing.  If the field doesn't exist, Fail(...).
func (m Map) Pop(fieldName string) *Value {
	v := m.PopMaybe(fieldName)
	if v == nil {
		m.Fail("missing field %q", fieldName)
	}
	return v
}

// Pop off a field for processing.  If the field doesn't exist, return nil.
func (m Map) PopMaybe(fieldName string) *Value {
	v, ok := m.inner[fieldName]
	if !ok {
		return nil
	}
	delete(m.inner, fieldName)
	return &Value{
		parent: m.source,
		fieldName: &fieldName,
		listIndex: -1,
		inner: v,
	}
}

// Pop off a field for processing.  If the field doesn't exist, do nothing.
// If the field does exist, call the given function to process the field's
// value further.
//     var m Map = ...
//     name := "default"
//     m.PopMaybeAnd("name", func(*v sx.Value) { name = v.String() })
func (m Map) PopMaybeAnd(fieldName string, run func(*Value)) {
	v := m.PopMaybe(fieldName)
	if v == nil {
		return
	}
	run(v)
}

type KeyValue struct {
	Key string
	Value *Value
}

// Pop off all fields and return them as a slice.
func (m Map) PopAll() []KeyValue {
	r := make([]KeyValue, 0, len(m.inner))
	for k, v := range m.inner {
		r = append(r, KeyValue{
			Key: k,
			Value: &Value{
				parent: m.source,
				fieldName: &k,
				listIndex: -1,
				inner: v,
			},
		})
	}
	return r
}

// If you want to ignore unknown fields, call this when you're done processing
// known fields.  If you don't, then Map(...) will automatically call Fail(...)
// if there are any unprocessed fields.
func (m Map) IgnoreRest() {
	for k := range m.inner {
		delete(m.inner, k)
	}
}

// Expect the value to be a list.  If it's not, Fail(...).
func (v *Value) List() List {
	l, ok := v.TryList()
	if !ok {
		v.Fail("expecting object, got %s", DescribeValueType(v.inner))
	}
	return l
}

// If the value is a list, returns (list, true), otherwise returns (..., false).
func (v *Value) TryList() (List, bool) {
	l, ok := v.inner.([]interface{})
	if !ok {
		return List{}, false
	}
	return List{
		source: v,
		inner: l,
	}, true
}

// Returns the length of the list.
func (l List) Len() int {
	return len(l.inner)
}

// Makes sure the list length is within the given min/max bounds (inclusive), then
// returns all the elements (each wrapped in a Value).
func (l List) AllB(min int, max int) []*Value {
	if len(l.inner) < min {
		l.Fail("list too short; minimum allowed elements: %d; got %d.", min, len(l.inner))
	}
	if len(l.inner) > max {
		l.Fail("list too long; maximum allowed elements: %d; got %d.", max, len(l.inner))
	}
	return l.All()
}

// Returns all the elements (each wrapped in a Value).
func (l List) All() []*Value {
	r := make([]*Value, 0, len(l.inner))
	for i, v := range l.inner {
		r = append(r, &Value{
			parent: l.source,
			fieldName: nil,
			listIndex: i,
			inner: v,
		})
	}
	return r
}

// A human-friendly description of the type.  Use this to help construct the
// Fail(...) message when some value doesn't have the type you expect.
func DescribeValueType(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case []interface{}:
		return "list"
	case map[string]interface{}:
		return "object"
	case bool:
		return "boolean"
	default:
		panic(fmt.Sprintf("bad value: %#v", v))
	}
}

