package filter

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
)

type FilterFunc func(*htevent.Event) bool
type Factory func() FilterFunc
type RuleBuilder func(l sx.List, args []*sx.Value) Factory


// If your filter function doesn't need any thread-local state, you can use this
// generic filter factory -- it just returns your 'filter' instance.
func StatelessFactory(filter FilterFunc) Factory {
	return func() FilterFunc { return filter }
}
