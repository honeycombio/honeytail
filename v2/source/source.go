package source

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"
)

type BuildFunc func(v *sx.Value, backfill bool) StartFunc
type StartFunc func(lineChannelChannel chan<- (<-chan string), abort <-chan struct{}) error
