package event

import (
    "time"
)

type Event struct {
	SampleRate int
	Timestamp time.Time
	Data map[string]interface{}
}
