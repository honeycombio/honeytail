package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"
	"github.com/yosuke-furukawa/json5/encoding/json5"
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

func LoadConfigFile(path string) (interface{}, error) {
	var raw interface{}

	if strings.HasSuffix(path, ".json") {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read file %q: %s", path, err)
		}

		err = json.Unmarshal(data, &raw)
		if err != nil {
			return nil, fmt.Errorf("%q: not valid JSON: %s", path, err)
		}
	} else if strings.HasSuffix(path, ".json5") {
		f, err := os.Open(path)
		defer f.Close()
		if err != nil {
			return nil, fmt.Errorf("unable to open file %q: %s", path, err)
		}

		err = json5.NewDecoder(f).Decode(&raw)
		if err != nil {
			return nil, fmt.Errorf("%q: not valid JSON5: %s", path, err)
		}
	} else {
		return nil, fmt.Errorf("%q: unsupported extension; expecting \".json\" or \".json5\".", path)
	}

	return raw, nil
}

func LoadConfigFileAndExtract(path string, extractor func(*sx.Value)) error {
	raw, err := LoadConfigFile(path)

	err = sx.Run(raw, extractor)
	if err != nil {
		return fmt.Errorf("%q: %s", path, err)
	}

	return nil
}
