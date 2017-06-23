package filter

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
)

type FilterFunc func(*htevent.Event) bool
type FilterFactory func() FilterFunc

func Build(v *sx.Value) FilterFactory {
	// If no filter config was provided, just return the default dummy config.
	if v == nil {
		filterFunc := func(event *htevent.Event) bool {
			return true
		}
		return func() FilterFunc {
			return filterFunc
		}
	}

	panic(v.Fail("sorry, filters not yet implemented"))
}
