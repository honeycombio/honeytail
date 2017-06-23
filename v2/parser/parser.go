package parser

import (
    sx "github.com/honeycombio/honeytail/v2/struct_extractor"
	"time"
	"fmt"
	"sync"
	"math/rand"
)

type BuildFunc func(v *sx.Value) StartFunc
type StartFunc func(lineChannelChannel <-chan (<-chan string), preSampleRate int, spawnWorkers func(Worker)) error
type Worker func(sendEvent SendEvent)
type SendEvent func(timestamp time.Time, data map[string]interface{})

func NewRand(preSampleRate int) *rand.Rand {
	if preSampleRate <= 0 {
		panic(fmt.Sprintf("bad preSampleRate %s", preSampleRate))
	}

	if preSampleRate == 1 {
		return nil
	}
	return rand.New(rand.NewSource(rand.Int63()))
}

func HandleEachLineChannel(lineChannelChannel <-chan (<-chan string), worker func(lineChannel <-chan string)) {
	linesWG := sync.WaitGroup{}
	for lineChannel := range lineChannelChannel {
		linesWG.Add(1)
		go func() {
			defer linesWG.Done()
			worker(lineChannel)
		}()
	}
	linesWG.Wait()
}
