// Package parsers provides an interface for different log parsing engines.
//
// Each module in here takes care of a specific log type, providing
// any necessary or relevant smarts for that style of logs.
package parsers

import (
	"sync"

	"github.com/honeycombio/honeytail/event"
)

type Parser interface {
	// Init does any initialization necessary for the module
	Init(options interface{}) error
	// ProcessLines consumes log lines from the lines channel and sends log events
	// to the send channel. It should add itself to the waitgroup and call
	// wg.Done() when it's finished processing lines
	ProcessLines(lines <-chan string, send chan<- event.Event, wg *sync.WaitGroup)
}
