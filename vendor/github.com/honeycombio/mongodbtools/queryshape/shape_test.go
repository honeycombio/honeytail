package queryshape_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/honeycombio/mongodbtools/logparser"
	"github.com/honeycombio/mongodbtools/queryshape"
)

func testQueryStringShape(t *testing.T, queryString, queryShape string) {
	q, err := logparser.ParseQuery(queryString)
	testOK(t, err)
	testEquals(t, queryshape.GetQueryShape(q), queryShape)
}

func TestSortedKeys(t *testing.T) {
	testQueryStringShape(t, "{ b: 1, c: 2, a: 3 }", `{ "a": 1, "b": 1, "c": 1 }`)
}

func TestFlattenedSlice(t *testing.T) {
	testQueryStringShape(t, "{ $in: [1, 2, 3] }", `{ "$in": 1 }`)
}

// helper function
func testEquals(t testing.TB, actual, expected interface{}, msg ...string) {
	if !reflect.DeepEqual(actual, expected) {
		message := strings.Join(msg, ", ")
		_, file, line, _ := runtime.Caller(2)

		t.Errorf(
			"%s:%d: %s -- actual(%T): %v, expected(%T): %v",
			filepath.Base(file),
			line,
			message,
			testDeref(actual),
			testDeref(actual),
			testDeref(expected),
			testDeref(expected),
		)
	}
}

func testDeref(v interface{}) interface{} {
	switch t := v.(type) {
	case *string:
		return fmt.Sprintf("*(%v)", *t)
	case *int64:
		return fmt.Sprintf("*(%v)", *t)
	case *float64:
		return fmt.Sprintf("*(%v)", *t)
	case *bool:
		return fmt.Sprintf("*(%v)", *t)
	default:
		return v
	}
}

func testOK(t testing.TB, err error, msg ...string) {
	if err != nil {
		message := strings.Join(msg, ", ")
		_, file, line, _ := runtime.Caller(2)

		t.Errorf("%s:%d: %s -- unexpected error: %s",
			filepath.Base(file),
			line,
			message,
			err.Error(),
		)
	}
}
