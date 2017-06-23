package wrapper

import (
	"github.com/Sirupsen/logrus"

	htparser "github.com/honeycombio/honeytail/v2/parser"
)

// If your log format is line-oriented, then just write a function that can
// parse a single line.  This wrapper will take care of the rest of the work.

// Given a log line, parse it into zero or more events and call 'sendEvent' on each one.
type LineParser func(line string, sendEvent htparser.SendEvent)

func Start(factory func() LineParser, lineChannelChannel <-chan <-chan string,
    preSampleRate int, spawnWorkers func(htparser.Worker)) error {

	// Merge all line channels into single channel
	combinedChannel := make(chan string, 1024)
	go func() {
		htparser.HandleEachLineChannel(lineChannelChannel, func(lineChannel <-chan string) {
			for line := range lineChannel {
				combinedChannel <- line
			}
		})
		close(combinedChannel)
	}()

	spawnWorkers(func(sendEvent htparser.SendEvent) {
		// Thread-locals, to avoid contention overhead.
		lineParser := factory()
		randObj := htparser.NewRand(preSampleRate)

		for line := range combinedChannel {
			if randObj != nil && randObj.Intn(preSampleRate) != 0 {
				continue
			}

			logrus.WithFields(logrus.Fields{
				"line": line,
			}).Debug("Attempting to process nginx log line")

			lineParser(line, sendEvent)
		}
	})

	return nil
}
