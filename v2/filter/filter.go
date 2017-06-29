package filter

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
)

// Given an Event, either perform modifications and return true, or return false
// to drop the event.
type FilterFunc func(*htevent.Event) bool

// Creates any thread-local state required by the filter, then returns the filter.
//
// When processing is performed by multiple threads, we'll call the factory to
// get a new filter function for each thread.
//
// If your filter function doesn't need any thread-local state, you can use
// the StatelessFactory helper to get a factory that just returns the same
// filter every time.
type TLFactory func() FilterFunc

// Given filter config info, returns a filter factory.  The arguments are in
// 'args'.  'l' is only provided so you can use 'l.Fail(...)' if something's
// wrong with the config (e.g. incorrect number of arguments).
type RuleBuilder func(l sx.List, args []*sx.Value) TLFactory

// Helper function that creates a factory that returns the same filter every time.
// Use this when you don't need any thread-local state.
func StatelessFactory(filter FilterFunc) TLFactory {
	return func() FilterFunc { return filter }
}
