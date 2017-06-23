package files

import (
    "fmt"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

    "github.com/honeycombio/honeytail/tail"
    htsource "github.com/honeycombio/honeytail/v2/source"
	"errors"
)

func Build(v *sx.Value, backfill bool) htsource.StartFunc {
    l := v.List()
    files := make([]string, 0, l.Len())
    for _, e := range l.All() {
        files = append(files, e.String())
    }

    return func(lineChannelChannel chan<- (<-chan string), abort <-chan struct{}) error {
        var options = tail.TailOptions{
            ReadFrom: "last",
            Stop: false,
        }
        if backfill {
            options.ReadFrom = "beginning"
            options.Stop = true
        }

        var err error
        tc := tail.Config{
            Paths:   files,
            Type:    tail.RotateStyleSyslog,
            Options: options,
        }

        lineChannels, err := tail.GetEntries(tc, abort)
        if err != nil {
            return fmt.Errorf("while trying to read files: %s", err)
        }

		if len(lineChannels) == 0 {
			return errors.New("no files to read")
		}

        go func() {
            for _, lineChannel := range lineChannels {
                lineChannelChannel <- lineChannel
            }
			close(lineChannelChannel)
        }()
        return nil
    }
}
