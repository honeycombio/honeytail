package httprequestline

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htfilter "github.com/honeycombio/honeytail/v2/filter"
	"github.com/honeycombio/urlshaper"
	"strings"
	"sort"
)

func Rule(l sx.List, args []*sx.Value) htfilter.Factory {
	if len(args) < 1 || len(args) > 2 {
		l.Fail("expecting 1 or 2 arguments, got %d.", len(args))
	}

	fieldName := args[0].String()

	fieldNamePrefix := ""
	parser := urlshaper.Parser{}
	extractQueryArgs := map[string]bool{}

	if len(args) == 2 {

		args[1].Map(func(m sx.Map) {
			m.PopMaybeAnd("field_name_prefix", func(v *sx.Value) {
				fieldNamePrefix = v.String()
			})

			m.PopMaybeAnd("path_patterns", func(v *sx.Value) {
				for _, patternV := range v.List().All() {
					patternS := patternV.String()
					pattern := &urlshaper.Pattern{Pat: patternS}
					err := pattern.Compile()
					if err != nil {
						patternV.Fail("bad path pattern %q: %s", patternS, err)
					}
					parser.Patterns = append(parser.Patterns, pattern)
				}
			})

			m.PopMaybeAnd("extract_query_args", func(v *sx.Value) {
				if l, ok := v.TryList(); ok {
					for _, elemV := range l.All() {
						extractQueryArgs[elemV.String()] = true
					}
				} else if s, ok := v.TryString(); ok {
					if s != "all" {
						v.Fail("expecting \"all\" or a list, got %q", s)
					}
					extractQueryArgs = nil
				} else {
					v.Fail("expecting \"all\" or a list")
				}
			})
		})
	}

	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		val, ok := event.Data[fieldName]
		if ok {
			s, ok := val.(string)
			if ok {
				extractSubfields(s, event.Data, fieldNamePrefix+fieldName, &parser, extractQueryArgs)
			}
		}
		return true
	})
}

func extractSubfields(val string, data map[string]interface{}, prefix string, parser *urlshaper.Parser, extractQueryArgs map[string]bool) {
	// start by splitting out method, uri, and version
	parts := strings.Split(val, " ")
	var path string
	if len(parts) == 3 {
		// treat it as METHOD /path HTTP/1.X
		data[prefix+"_method"] = parts[0]
		data[prefix+"_protocol_version"] = parts[2]
		path = parts[1]
	} else {
		// treat it as just the /path
		path = parts[0]
	}
	// next up, get all the goodies out of the path
	res, err := parser.Parse(path)
	if err != nil {
		// couldn't parse it, just pass along the event
		return
	}
	data[prefix+"_uri"] = res.URI
	data[prefix+"_path"] = res.Path
	if res.Query != "" {
		data[prefix+"_query"] = res.Query
	}
	for k, v := range res.QueryFields {
		if extractQueryArgs == nil || extractQueryArgs[k] {
			if len(v) > 1 {
				sort.Strings(v)
			}
			data[prefix+"_query_"+k] = strings.Join(v, ", ")
		}
	}
	for k, v := range res.PathFields {
		data[prefix+"_path_"+k] = v[0]
	}
	data[prefix+"_shape"] = res.Shape
	data[prefix+"_pathshape"] = res.PathShape
	if res.QueryShape != "" {
		data[prefix+"_queryshape"] = res.QueryShape
	}
}
