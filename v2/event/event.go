package event

import (
    "time"
)

type Event struct {
	SampleRate uint
	Timestamp time.Time
	Data map[string]interface{}
}
