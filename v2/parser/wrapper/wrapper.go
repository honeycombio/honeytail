package wrapper

import (
	htparser "github.com/honeycombio/honeytail/v2/parser"
)

// If your log format is line-oriented, then just write a function that can
// parse a single line.  This wrapper will take care of the rest of the work.

// Perform one-time setup.
type SetupLineParser func() (LineParserFactory, error)

// Perform thread-local setup.
type LineParserFactory func() LineParser

// Given a log line, parse it into zero or more events and call 'sendEvent' on each one.
type LineParser func(line string, sendEvent htparser.SendEvent)

func LineParserWrapper(setupLineParser SetupLineParser) htparser.BuildFunc {
	return func(channelSize int) (func(), htparser.PreParser, htparser.Parser, error) {
		lineParserFactory, err := setupLineParser()
		if err != nil {
			return nil, nil, nil, err
		}

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
			lineParser := lineParserFactory()  // Thread-local, to avoid contention overhead.
			for line := range combinedChannel {
				lineParser(line, sendEvent)
			}
		}

		return closeChannel, preParser, parser, nil
	}
}
