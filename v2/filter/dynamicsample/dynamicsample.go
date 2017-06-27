package filter

import (
	"math/rand"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/dynsampler-go"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htfilter "github.com/honeycombio/honeytail/v2/filter"
	"fmt"
)

func Rule(l sx.List, args []*sx.Value) htfilter.Factory {
	if len(args) < 2 || len(args) > 3 {
		l.Fail("expecting 2 or 3 arguments, got %d.", len(args))
	}
	goalRate := args[0].Int32B(1, 1000000)

	fieldNamesV := args[1].List()
	fieldNames := make([]string, 0, fieldNamesV.Len())
	if fieldNamesV.Len() < 1 {
		fieldNamesV.Fail("must contain at least one entry")
	}
	for _, fieldNameV := range fieldNamesV.All() {
		fieldNames = append(fieldNames, fieldNameV.String())
	}

	var windowSec int = 30
	if len(args) == 3 {
		args[2].Map(func(m sx.Map) {
			windowSec = int(m.Pop("window_sec").Int32B(1, 1000000))
		})
	}

	sampler := &dynsampler.AvgSampleWithMin{
		GoalSampleRate:    int(goalRate),
		ClearFrequencySec: windowSec,
	}
	if err := sampler.Start(); err != nil {
		// TODO: should we panic?
		logrus.WithField("error", err).Fatal("dynsampler failed to start")
	}

	return func() htfilter.FilterFunc {
		randObj := *rand.New(rand.NewSource(rand.Int63()))  // Thread-local, to avoid contention overhead
		return func(event *htevent.Event) bool {
			key := makeKey(event.Data, fieldNames)
			rate := sampler.GetSampleRate(key)
			if rate < 0 {
				panic(fmt.Sprintf("bad rate: %d", rate))
			}

			keep := randObj.Intn(rate) == 0
			if keep {
				event.SampleRate *= uint(rate)
			}
			return keep
		}
	}
}

func makeKey(data map[string]interface{}, fieldNames []string) string {
	fieldValues := make([]string, len(fieldNames))
	for i, field := range fieldNames {
		if val, ok := data[field]; ok {
			switch val := val.(type) {
			case bool:
				fieldValues[i] = strconv.FormatBool(val)
			case int64:
				fieldValues[i] = strconv.FormatInt(val, 10)
			case float64:
				fieldValues[i] = strconv.FormatFloat(val, 'E', -1, 64)
			case string:
				fieldValues[i] = val
			default:
				fieldValues[i] = "" // skip it
			}
		}
	}
	return strings.Join(fieldValues, "_")
}
