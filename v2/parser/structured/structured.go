package structured

import (
	"sync"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htparser "github.com/honeycombio/honeytail/v2/parser"
)

// Implement this package's ConfigureFunc if your parser follows the standard
// structure of pre-parse -> sample -> parse.  You can call ToStandardConfigureFunc
// to turn it into a standard ConfigureFunc.

// Check the configuration in 'v'.  Don't check anything about the outside world yet.
type ConfigureFunc func(v *sx.Value) SetupFunc

// Check stuff about the world and return an error if something is wrong.  For example,
// if there's a file you require that isn't present.
type SetupFunc func() (BuildFunc, error)

// Create an intermediary channel for the pre-parser to send entries to the parser, then
// return the necessary Component structure.
type BuildFunc func(channelSize int) Components

type Components struct {
	// Close the intermediary channel created by BuildFunc.  This will be called when
	// all the pre-parser threads have completed.
	CloseChannel func()
	// Do minimal parsing to identify individual events.  This is also where
	// pre-sampling is done.
	PreParser PreParser
	// Parse events based on objects passed from the pre-parser.
	Parser Parser
}

// Do the minimum amount of parsing to identify a group of lines that is a single event,
// check the given sampler, then write the barely-parsed event to the intermediary channel
// created in 'BuildFunc'.
type PreParser func(lineChannel <-chan string, sampler htparser.Sampler)

// Read from the intermediary channel created in 'BuildFunc', parse the event's fields, and
// pass the result to 'sendEvent'
type Parser func(sendEvent htparser.SendEvent)

func ToStandardConfigureFunc(configureFunc ConfigureFunc) htparser.ConfigureFunc {
	return func(v *sx.Value) htparser.SetupFunc {
		setupFunc := configureFunc(v)
		return ToStandardSetupFunc(setupFunc)
	}
}

func ToStandardSetupFunc(setupFunc SetupFunc) htparser.SetupFunc {
	return func() (htparser.StartFunc, error) {
		startFunc, err := setupFunc()
		if err != nil {
			return nil, err
		}
		return ToStandardStartFunc(startFunc), nil
	}
}

func ToStandardStartFunc(buildFunc BuildFunc) htparser.StartFunc {
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
					sampler := samplerTLFactory() // Thread-local, to avoid contention overhead.
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
				sendEvent := sendEventTLFactory() // Thread-local, to avoid contention overhead.
				components.Parser(sendEvent)
			}()
		}
	}
}
