package parser

import (
	"time"
	"sync"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"
)

// Check the configuration in 'v'.  Don't check anything about the outside world yet.
type ConfigureFunc func(v *sx.Value) SetupFunc

// Check stuff about the world and return an error if something is wrong.  For example,
// if there's a file you require that isn't present.
type SetupFunc func() (StartFunc, error)

// Start a process that will read from the given streams of log lines, parse them into events,
// then send them via 'sendEvent'.
//
// The caller will wait on 'doneWG' to determine when the parsing is complete.
//
// See parser/structured.NewStartFunc" for a helper that makes it easier to define a StartFunc.
type StartFunc func(
	numThreads int,
	lineChannelChannel <-chan (<-chan string),
	samplerTLFactory SamplerTLFactory,
	sendEventTLFactory SendEventTLFactory,
	doneWG *sync.WaitGroup)

type SendEvent func(timestamp time.Time, data map[string]interface{})

// Performs thread-local setup.
type SendEventTLFactory func() SendEvent

type Sampler interface {
	ShouldKeep() bool
}

// Performs thread-local setup.
type SamplerTLFactory func() Sampler
