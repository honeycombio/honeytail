package structured

import (
	"sync"

	htparser "github.com/honeycombio/honeytail/v2/parser"
)

// An easier way to create a 'parser.StartFunc'.  If your parsing pipeline follows the standard
// structure of pre-parse -> sample -> parse, then use this.  It will take care of creating
// all the threads for you.
//
// If your log format has one entry per line, use NewStartFuncForLineParser.
func NewStartFunc(buildFunc BuildFunc) htparser.StartFunc {
	return func(numThreads int, lineChannelChannel <-chan (<-chan string), samplerTLFactory htparser.SamplerTLFactory,
		sendEventTLFactory htparser.SendEventTLFactory, doneWG *sync.WaitGroup) {

		channelSize := 2 * numThreads
		components := buildFunc(channelSize)

		// New thread to spawn a pre-parsers for each new line channel.
		go func() {
			wg := sync.WaitGroup{}
			for lineChannel := range lineChannelChannel {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sampler := samplerTLFactory()  // Thread-local, to avoid contention overhead.
					components.PreParser(lineChannel, sampler)
				}()
			}
			wg.Wait()
			components.CloseChannel()
		}()

		// Start parser threads.
		for i := 0; i < numThreads; i++ {
			doneWG.Add(1)
			go func() {
				defer doneWG.Done()
				sendEvent := sendEventTLFactory()  // Thread-local, to avoid contention overhead.
				components.Parser(sendEvent)
			}()
		}
	}
}

// Build the components of a structured parser.
type BuildFunc func(channelSize int) Components

type Components struct {
	// The pre-parser and parser communicate via an intermediary channel.  This function should
	// close that channel.  It will be automatically called when all spawned pre-parser threads
	// have completed.
	CloseChannel func()
	// Do minimal parsing to identify individual events.  This is also where pre-sampling is done.
	PreParser    PreParser
	// Parse events based on objects passed from the pre-parser.
	Parser       Parser
}

// Do the minimum amount of parsing to identify a group of lines that is a single event,
// check the given sampler, then write the barely-parsed event to the intermediary channel
// created in 'BuildFunc'.
type PreParser func(lineChannel <-chan string, sampler htparser.Sampler)

// Read from the intermediary channel created in 'BuildFunc', parse the event's fields, and
// pass the result to 'sendEvent'
type Parser func(sendEvent htparser.SendEvent)

// If your log format has one entry per line, this wrapper allows you to only define
// the function to process a single line.  This takes care of the rest of the work of
// defining a full log parser.
func NewStartFuncForLineParser(lineParserTLFactory LineParserTLFactory) htparser.StartFunc {
	return NewStartFunc(NewBuildFuncForLineParser(lineParserTLFactory))
}

func NewBuildFuncForLineParser(lineParserTLFactory LineParserTLFactory) BuildFunc {
	return func(channelSize int) Components {
		combinedChannel := make(chan string, channelSize)
		closeChannel := func() { close(combinedChannel) }

		preParser := func(lineChannel <-chan string, sampler htparser.Sampler) {
			// Just reads from the separate log channels and writes to the combined log channel.
			for line := range lineChannel {
				if sampler.ShouldKeep() {
					combinedChannel <- line
				}
			}
		}

		parser := func(sendEvent htparser.SendEvent) {
			lineParser := lineParserTLFactory()  // Thread-local, to avoid contention overhead.
			for line := range combinedChannel {
				lineParser(line, sendEvent)
			}
		}

		return Components{closeChannel, preParser, parser}
	}
}

// Performs thread-local setup.
type LineParserTLFactory func() LineParser

// Given a log line, parse it into zero or more events and call 'sendEvent' on each one.
type LineParser func(line string, sendEvent htparser.SendEvent)

