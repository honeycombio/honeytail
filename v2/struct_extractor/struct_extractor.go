package struct_extractor

import (
	"fmt"
	"math"
	"strings"
	"errors"
)

type Value struct {
	parent    *Value
	fieldName *string
	listIndex int
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

func (v *Value) Fail(format string, args ...interface{}) interface{} {
	parts := []string{ fmt.Sprintf(format, args...) }

	// Append all the path components.
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

func (m Map) Fail(format string, args ...interface{}) {
	m.source.Fail(format, args...)
}

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

func (v *Value) Int32() int32 {
	return int32(v.intB(math.MinInt32, math.MaxInt32))
}

func (v *Value) Int32B(min int32, max int32) int32 {
	return int32(v.intB(int64(min), int64(max)))
}

func (v *Value) UInt32() uint32 {
	return uint32(v.intB(0, math.MaxUint32))
}

func (v *Value) UInt32B(min uint32, max uint32) uint32 {
	return uint32(v.intB(int64(min), int64(max)))
}

func (v *Value) String() string {
	s, ok := v.inner.(string)
	if !ok {
		v.Fail("expecting string, got %s", DescribeValueType(v.inner))
	}
	return s
}

func (v *Value) TryString() (string, bool) {
	s, ok := v.inner.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func (v *Value) TaggedUnion() (string, *Value) {
	m := v.RawMap()
	kvs := m.PopAll()
	if len(kvs) != 1 {
		m.Fail("expecting a single entry, got %d.", len(kvs))
	}
	return kvs[0].Key, kvs[0].Value
}

func (v *Value) Any() interface{} {
	return v.inner
}

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

func (v *Value) RawMap() Map {
	m, ok := v.TryRawMap()
	if !ok {
		v.Fail("expecting object, got %s", DescribeValueType(v.inner))
	}
	return m
}

func (v *Value) Map(f func(m Map)) {
	m := v.RawMap()
	f(m)
	m.Done()
}

func (m Map) Done() {
	// Make sure all fields were popped.
	if len(m.inner) > 0 {
		// Get the minimum field name.  This way the error message is deterministic.
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

func (m Map) Pop(fieldName string) *Value {
	v := m.PopMaybe(fieldName)
	if v == nil {
		m.Fail("missing field %q", fieldName)
	}
	return v
}

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

func (m Map) IgnoreRest() {
	for k := range m.inner {
		delete(m.inner, k)
	}
}

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

func (v *Value) List() List {
	l, ok := v.TryList()
	if !ok {
		v.Fail("expecting object, got %s", DescribeValueType(v.inner))
	}
	return l
}

func (l List) Len() int {
	return len(l.inner)
}

func (l List) AllB(min int, max int) []*Value {
	if len(l.inner) < min {
		l.Fail("list too short; minimum allowed elements: %d; got %d.", min, len(l.inner))
	}
	if len(l.inner) > max {
		l.Fail("list too long; maximum allowed elements: %d; got %d.", max, len(l.inner))
	}
	return l.All()
}

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

