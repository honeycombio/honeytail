package util

import (
	"fmt"
	"os"
    "regexp"

	"github.com/yosuke-furukawa/json5/encoding/json5"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"
)

// ExtRegexp is a Regexp with one additional method to make it easier to work
// with named groups
type ExtRegexp struct {
	*regexp.Regexp
}

// FindStringSubmatchMap behaves the same as FindStringSubmatch except instead
// of a list of matches with the names separate, it returns the full match and a
// map of named submatches
func (r *ExtRegexp) FindStringSubmatchMap(s string) (string, map[string]string) {
	match := r.FindStringSubmatch(s)
	if match == nil {
		return "", nil
	}

	captures := make(map[string]string)
	for i, name := range r.SubexpNames() {
		if i == 0 {
			continue
		}
		if name != "" {
			// ignore unnamed matches
			captures[name] = match[i]
		}
	}
	return match[0], captures
}

func LoadTomlFileAndExtract(path string, extractor func(*sx.Value)) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("unable to read file %q: %s", path, err)
	}

	var raw interface{}
	err = json5.NewDecoder(f).Decode(&raw)
	if err != nil {
		return fmt.Errorf("%q: not valid JSON5: %s", path, err)
	}

	err = sx.Run(raw, extractor)
	if err != nil {
		return fmt.Errorf("%q: %s", path, err)
	}

	return nil
}
