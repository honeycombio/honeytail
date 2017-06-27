package parser

import (
    sx "github.com/honeycombio/honeytail/v2/struct_extractor"
	"time"
)

// Check the parser's configuration in 'v'.  Don't check anything about the outside world yet.
type ConfigureFunc func(v *sx.Value) BuildFunc

// Try and build a parser.  If there's something about the outside world that isn't what you
// expect, return an error.
type BuildFunc func(channelSize int) (func(), PreParser, Parser, error)

// Do the minimum amount of parsing to identify a group of lines that is a single event,
// apply the given sampler, then write the barely-parsed event to the intermediary channel
// created in 'BuildFunc'.
type PreParser func(lineChannel <-chan string, sampler Sampler)

// Read from the intermediary channel created in 'BuildFunc', parse the event's fields, and
// pass the result to 'sendEvent'
type Parser func(sendEvent SendEvent)
type SendEvent func(timestamp time.Time, data map[string]interface{})

type Sampler interface {
	ShouldKeep() bool
}
