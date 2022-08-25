package httime

import (
	"math"
	"testing"
	"time"

	"github.com/honeycombio/honeytail/httime/httimetest"
)

func init() {
	DefaultNower = &httimetest.FakeNower{}
}

func TestFormat(t *testing.T) {
	tsts := []struct {
		strftime string
		expected string
	}{
		{"%Y-%m-%d %H:%M:%S", "2006-01-02 15:04:05"},
		{"%Y-%m-%d %H:%M", "2006-01-02 15:04"},
		{"%Y-%m-%d %k:%M", "2006-01-02 15:04"},
		{"%m/%d/%y %I:%M %p", "01/02/06 03:04 PM"},
		{"%m/%d/%y %I:%M %P%t%z", "01/02/06 03:04 pm\t-0700"},
		{"%a %B %d %C %r", "Mon January 02 06 03:04:05 PM"},
		{"%c %G %g %O %u %V %w %X", "       "},
		{"%H:%M:%S.%f", "15:04:05.999"},
	}

	for _, tt := range tsts {
		gotime := convertTimeFormat(tt.strftime)
		if gotime != tt.expected {
			t.Errorf("strftime format '%s' was parsed to go time '%s', expected '%s'",
				tt.strftime, gotime, tt.expected)
		}
	}
}

type testTimestamp struct {
	format        string         // the format this test's time is in
	fieldName     string         // the field in the map containing the time
	input         interface{}    // the value corresponding to the fieldName
	tz            *time.Location // the expected time zone
	auto          bool           // whether the input should be parsable even without specifying format/fieldName
	expected      time.Time      // the expected time object to get back
	diffThreshold time.Duration  // the epsilon for which different times are the same, to handle floats
}

var utc = time.UTC
var pacific, _ = time.LoadLocation("America/Los_Angeles")

var tts = []testTimestamp{
	{
		format:        "2006-01-02 15:04:05.999999999 -0700 MST",
		fieldName:     "time",
		input:         "2014-04-10 19:57:38.123456789 -0800 PST",
		tz:            utc,
		auto:          true,
		expected:      time.Unix(1397188658, 123456789),
		diffThreshold: 0,
	},
	{
		format:        time.RFC3339Nano,
		fieldName:     "timestamp",
		input:         "2014-04-10T19:57:38.123456789-08:00",
		tz:            utc,
		auto:          true,
		expected:      time.Unix(1397188658, 123456789),
		diffThreshold: 0,
	},
	{
		format:        time.RFC3339,
		fieldName:     "Date",
		input:         "2014-04-10T19:57:38-08:00",
		tz:            utc,
		auto:          true,
		expected:      time.Unix(1397188658, 0),
		diffThreshold: 0,
	},
	{
		format:        time.RFC3339,
		fieldName:     "Date",
		input:         "2014-04-10T19:57:38Z",
		tz:            utc,
		auto:          true,
		expected:      time.Unix(1397159858, 0),
		diffThreshold: 0,
	},
	{
		format:        time.RubyDate,
		fieldName:     "datetime",
		input:         "Thu Apr 10 19:57:38.123456789 -0800 2014",
		tz:            utc,
		auto:          true,
		expected:      time.Unix(1397188658, 123456789),
		diffThreshold: 0,
	},
	{
		format:        "%Y-%m-%d %H:%M",
		fieldName:     "time",
		input:         "2014-07-30 07:02",
		tz:            utc,
		expected:      time.Unix(1406703720, 0),
		diffThreshold: 0,
	},
	{
		format:        "%Y-%m-%d %H:%M",
		fieldName:     "time",
		input:         "2014-07-30 07:02",
		tz:            pacific,
		expected:      time.Unix(1406728920, 0),
		diffThreshold: 0,
	},
	{
		format:        "%Y-%m-%d %k:%M", // check trailing space behavior
		fieldName:     "time",
		input:         "2014-07-30  7:02",
		tz:            utc,
		expected:      time.Unix(1406703720, 0),
		diffThreshold: 0,
	},
	{
		format:        "%Y-%m-%d %H:%M:%S",
		fieldName:     "time",
		input:         "2014-07-30 07:02:15",
		tz:            utc,
		expected:      time.Unix(1406703735, 0),
		diffThreshold: 0,
	},
	{
		format:        UnixTimestampFmt,
		fieldName:     "time",
		input:         "1440116565",
		tz:            utc,
		expected:      time.Unix(1440116565, 0),
		diffThreshold: 0,
	},
	{
		format:        UnixTimestampFmt,
		fieldName:     "time",
		input:         1440116565,
		tz:            utc,
		expected:      time.Unix(1440116565, 0),
		diffThreshold: 0,
	},
	// millis
	{
		format:        UnixTimestampFmt,
		fieldName:     "time",
		input:         "1440116565.123",
		tz:            utc,
		expected:      time.Unix(1440116565, 123000000),
		diffThreshold: time.Millisecond,
	},
	{
		format:        UnixTimestampFmtAlt,
		fieldName:     "time",
		input:         "1440116565.123456",
		tz:            utc,
		expected:      time.Unix(1440116565, 123456000),
		diffThreshold: time.Microsecond,
	},
	{
		format:        UnixTimestampFmtTxt,
		fieldName:     "time",
		input:         "1440116565.12345678",
		tz:            utc,
		expected:      time.Unix(1440116565, 123456780),
		diffThreshold: 100 * time.Nanosecond,
	},
	{
		format:        "%Y-%m-%d %z",
		input:         "2014-04-10 -0700",
		tz:            utc,
		fieldName:     "time",
		expected:      time.Unix(1397113200, 0),
		diffThreshold: 0,
	},
	{
		format:        "",
		input:         "1538860697500",
		tz:            utc,
		fieldName:     "timestamp",
		expected:      time.Unix(1538860697, 500000000),
		diffThreshold: 0,
	},
}

func TestGetTimestampValid(t *testing.T) {
	for i, tTimeSet := range tts {
		Location = tTimeSet.tz
		if tTimeSet.auto {
			fields := map[string]interface{}{tTimeSet.fieldName: tTimeSet.input}
			resp := GetTimestamp(fields, "", "")
			if !resp.Equal(tTimeSet.expected) {
				t.Errorf("time %d: should've been parsed automatically, without required config", i)
			}
			if len(fields) != 0 {
				t.Error("time field should have been deleted, but was not")
			}
		}

		fields := map[string]interface{}{tTimeSet.fieldName: tTimeSet.input}
		resp := GetTimestamp(fields, tTimeSet.fieldName, tTimeSet.format)
		if !approxEqual(resp, tTimeSet.expected, tTimeSet.diffThreshold) {
			t.Errorf("time %d: resp time %s didn't match expected time %s", i, resp, tTimeSet.expected)
		}
		if len(fields) != 0 {
			t.Error("time field should have been deleted, but was not")
		}
	}
}

func approxEqual(t1, t2 time.Time, threshold time.Duration) bool {
	diff := int64(math.Abs(float64(t1.Sub(t2))))
	return diff <= int64(threshold)
}

func TestGetTimestampInvalid(t *testing.T) {
	// time field missing
	resp := GetTimestamp(map[string]interface{}{"noTimeField": "not used"}, "", "")
	if !resp.Equal(Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, Now())
	}
	// time field unparsable
	resp = GetTimestamp(map[string]interface{}{"time": "not a valid date"}, "", "")
	if !resp.Equal(Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, Now())
	}
	fields := map[string]interface{}{"time": "not a valid date", "key": "value"}
	resp = GetTimestamp(fields, "", "")
	if !resp.Equal(Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, Now())
	}
	if len(fields) != 2 {
		t.Error("fields was modified despite not having a valid timestamp")
	}
}

func TestGetTimestampCustomFormat(t *testing.T) {
	weirdFormat := "Mon // 02 ---- Jan ... 06 15:04:05 -0700"

	testStr := "Mon // 09 ---- Aug ... 10 15:34:56 -0800"
	expected := time.Date(2010, 8, 9, 15, 34, 56, 0, time.FixedZone("PST", -28800))

	// with just Format defined
	resp := GetTimestamp(map[string]interface{}{"timestamp": testStr}, "", weirdFormat)
	if !resp.Equal(expected) {
		t.Errorf("resp time %s didn't match expected time %s", resp, expected)
	}

	// with just TimeFieldName defined -- use one of the expected/fallback
	// formats
	resp = GetTimestamp(map[string]interface{}{"funkyTime": expected.Format(time.RubyDate)}, "funkyTime", "")
	if !resp.Equal(expected) {
		t.Errorf("resp time %s didn't match expected time %s", resp, expected)
	}
	resp = GetTimestamp(map[string]interface{}{"funkyTime": testStr}, "funkyTime", weirdFormat)
	if !resp.Equal(expected) {
		t.Errorf("resp time %s didn't match expected time %s", resp, expected)
	}
	// don't parse the "time" field if we're told to look for time in "funkyTime"
	resp = GetTimestamp(map[string]interface{}{"time": "2014-04-10 19:57:38.123456789 -0800 PST"}, "funkyTime", weirdFormat)
	if !resp.Equal(Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, Now())
	}
}

func TestGetTimestampTypeTime(t *testing.T) {
	expected := time.Now()
	resp := GetTimestamp(map[string]interface{}{"real_time_key": expected}, "real_time_key", "")
	if !resp.Equal(expected) {
		t.Errorf("resp time %s didn't match expected time %s", resp, expected)
	}
}

func TestCommaInTimestamp(t *testing.T) {
	commaTimes := []testTimestamp{
		{ // test commas as the fractional portion separator
			format:    "2006-01-02 15:04:05,999999999 -0700 MST",
			fieldName: "time",
			input:     "2014-03-10 12:57:38,123456789 -0700 PDT",
			expected:  time.Unix(1394481458, 123456789),
		},
		{
			format:    "2006-01-02 15:04:05.999999999 -0700 MST",
			fieldName: "time",
			input:     "2014-03-10 12:57:38,123456789 -0700 PDT",
			expected:  time.Unix(1394481458, 123456789),
		},
	}
	for i, tTimeSet := range commaTimes {
		expectedTime := tTimeSet.expected
		resp := GetTimestamp(map[string]interface{}{tTimeSet.fieldName: tTimeSet.input}, tTimeSet.fieldName, tTimeSet.format)
		if !resp.Equal(expectedTime) {
			t.Errorf("time %d: resp time %s didn't match expected time %s", i, resp, expectedTime)
		}
	}

}
