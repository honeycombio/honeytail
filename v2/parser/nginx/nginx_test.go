package nginx

import (
	"reflect"
	"testing"
	"time"
)

func TestGetTimestamp(t *testing.T) {
	zeroTs := time.Time{}
	octoberTs, _ := time.Parse(commonLogFormatTimeLayout, "08/Oct/2015:00:26:26 +0000")

	testCases := []struct {
		input     map[string]interface{}
		postMunge map[string]interface{}
		retval    time.Time
	}{
		{ //well formatted time_local
			input: map[string]interface{}{
				"foo":        "bar",
				"time_local": "08/Oct/2015:00:26:26 +0000",
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: octoberTs,
		},
		{ //well formatted time_iso
			input: map[string]interface{}{
				"foo":          "bar",
				"time_iso8601": "2015-10-08T00:26:26-00:00",
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: octoberTs,
		},
		{ //broken formatted time_local
			input: map[string]interface{}{
				"foo":        "bar",
				"time_local": "08aoeu00:26:26 +0000",
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: zeroTs,
		},
		{ //broken formatted time_iso
			input: map[string]interface{}{
				"foo":          "bar",
				"time_iso8601": "2015-aoeu00:00",
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: zeroTs,
		},
		{ //non-string formatted time_local
			input: map[string]interface{}{
				"foo":        "bar",
				"time_local": 1234,
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: zeroTs,
		},
		{ //non-string formatted time_iso
			input: map[string]interface{}{
				"foo":          "bar",
				"time_iso8601": 1234,
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: zeroTs,
		},
		{ //missing time field
			input: map[string]interface{}{
				"foo": "bar",
			},
			postMunge: map[string]interface{}{
				"foo": "bar",
			},
			retval: zeroTs,
		},
	}
	for _, tc := range testCases {
		res := getTimestamp(tc.input)
		if !reflect.DeepEqual(tc.input, tc.postMunge) {
			t.Errorf("didn't remove time field: %v", tc.input)
		}
		if !reflect.DeepEqual(res, tc.retval) {
			t.Errorf("got wrong time. expected %v got %v", tc.retval, res)
		}
	}
}
