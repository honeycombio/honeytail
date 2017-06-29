package test_utils

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_structured "github.com/honeycombio/honeytail/v2/parser/structured"
)

// Helper function for parser tests.  Just make sure a given input stream yields the expected
// list of events.
//
// If there are events that make it through the pre-parser, but are dropped by the parser, those
// events should be represented by 'nil' entries in 'expectedOutput'.
func Check(t *testing.T, buildFunc htparser_structured.BuildFunc, input []string, expectedOutput []*htevent.Event) {
	run := func (name string, predicate func(int) bool) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			samplerReturnValues, sampledExpectedOutput := buildSamplerArrays(expectedOutput, predicate)
			sampler := &controlledSampler{0, samplerReturnValues}
			CheckSampled(t, buildFunc, input, sampledExpectedOutput, sampler)
			if sampler.index != len(sampler.returnValues) {
				t.Fatalf("sampler wasn't called enough times; expected %d, got %d", len(sampler.returnValues), sampler.index)
			}
		})
	}

	// Test with sampling disabled (predicate returns true for all events).  This makes sure
	// all expected output appears.
	run("all", func(_ int) bool { return true })

	// Try with different sampling.  Makes sure the parser is using the sampler correctly.
	run("none", func(_ int) bool { return false })
	run("evens", func(i int) bool { return i % 2 == 0 })
	run("odds", func(i int) bool { return i % 2 == 1 })
	run("third", func(i int) bool { return i % 3 == 0 })
	run("third+1", func(i int) bool { return i % 3 == 1 })
	run("third+2", func(i int) bool { return i % 3 == 2 })
}

func CheckSampled(
	t *testing.T,
	buildFunc htparser_structured.BuildFunc,
	input []string,
	expectedOutput []*htevent.Event,
	sampler htparser.Sampler) {

	lineChannel := make(chan string, 10)
	components := buildFunc(10)

	go func() {
		for _, line := range input {
			lineChannel <- line
		}
		close(lineChannel)
	}()

	go func() {
		components.PreParser(lineChannel, sampler)
		components.CloseChannel()
	}()

	output := make([]*htevent.Event, 0, len(expectedOutput))
	sendEvent := func(timestamp time.Time, data map[string]interface{}) {
		output = append(output, &htevent.Event{
			Timestamp: timestamp,
			Data: data,
		})
	}

	expectedOutputWithoutNils := make([]*htevent.Event, 0, len(expectedOutput))
	for _, event := range expectedOutput {
		if event != nil {
			expectedOutputWithoutNils = append(expectedOutputWithoutNils, event)
		}
	}

	components.Parser(sendEvent)

	if !reflect.DeepEqual(output, expectedOutputWithoutNils) {
		t.Fatalf("expected %s, got %s", spew.Sdump(expectedOutputWithoutNils), spew.Sdump(output))
	}
}

// Based on the given predicate, returns two things:
// - samplerReturnValues: An array of true/false values from the predicate.  The size of this array is the
//   size of 'expectedOutput'
// - sampledExpectedOutput: A subset of 'expectedOutput' with all the false entries filtered out.
func buildSamplerArrays(expectedOutput []*htevent.Event, predicate func(int) bool) ([]bool, []*htevent.Event) {
	samplerReturnValues := make([]bool, 0, len(expectedOutput))
	sampledExpectedOutput := make([]*htevent.Event, 0, len(expectedOutput))

	for i, event := range expectedOutput {
		shouldKeep := predicate(i)
		samplerReturnValues = append(samplerReturnValues, shouldKeep)
		if shouldKeep {
			sampledExpectedOutput = append(sampledExpectedOutput, event)
		}
	}
	return samplerReturnValues, sampledExpectedOutput
}

// A sampler that returns true/false according to our predetermined list of values.
type controlledSampler struct {
	index int
	returnValues []bool
}

func (cs *controlledSampler) ShouldKeep() bool {
	if cs.index >= len(cs.returnValues) {
		panic(fmt.Sprintf("there are %d return values, but the sampler was called too many times.", len(cs.returnValues)))
	}

	r := cs.returnValues[cs.index]
	cs.index++
	return r
}
