// Package parsers provides an interface for different log parsing engines.
//
// Each module in here takes care of a specific log type, providing
// any necessary or relevant smarts for that style of logs.
package parsers

import "github.com/honeycombio/honeytail/event"

type Parser interface {
	// Init does any initialization necessary for the module
	Init(options interface{}) error
	// ProcessLines consumes log lines from the lines channel and sends log events
	// to the send channel.
	ProcessLines(lines <-chan string, send chan<- event.Event)
	// log something about the parser's statistics (eg num events parsed)
	LogStats()
}
