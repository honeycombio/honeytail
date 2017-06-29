package line

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_structured "github.com/honeycombio/honeytail/v2/parser/structured"
)

// Implement this package's ConfigureFunc if your parser is line-based.  You can call
// ToStructuredConfigureFunc to turn it into a structured parser's ConfigureFunc.

// Check the parser's configuration in 'v'.  Don't check anything about the outside world yet.
type ConfigureFunc func(v *sx.Value) SetupFunc

// Check stuff about the world and issue an error if something is wrong.
type SetupFunc func() (ParserTLFactory, error)

// Given a log line, parse it into zero or more events and call 'sendEvent' on each one.
type Parser func(line string, sendEvent htparser.SendEvent)

// Performs thread-local setup.
type ParserTLFactory func() Parser

func ToStructuredConfigureFunc(f ConfigureFunc) htparser_structured.ConfigureFunc {
	return func(v *sx.Value) htparser_structured.SetupFunc {
		lineSetupFunc := f(v)
		return ToStructuredSetupFunc(lineSetupFunc)
	}
}

func ToStructuredSetupFunc(f SetupFunc) htparser_structured.SetupFunc {
	return func() (htparser_structured.BuildFunc, error) {
		factory, err := f()
		if err != nil {
			return nil, err
		}
		return ToStructuredBuildFunc(factory), nil
	}
}

func ToStructuredBuildFunc(f ParserTLFactory) htparser_structured.BuildFunc {
	return func(channelSize int) htparser_structured.Components {
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
			lineParser := f()  // Thread-local, to avoid contention overhead.
			for line := range combinedChannel {
				lineParser(line, sendEvent)
			}
		}

		return htparser_structured.Components{closeChannel, preParser, parser}
	}
}
