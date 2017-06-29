package registry

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htevent "github.com/honeycombio/honeytail/v2/event"
	htfilter "github.com/honeycombio/honeytail/v2/filter"
	htfilter_dynamicsample "github.com/honeycombio/honeytail/v2/filter/dynamicsample"
	htfilter_httprequestline "github.com/honeycombio/honeytail/v2/filter/httprequestline"
)

func Build(v *sx.Value) htfilter.FilterTLFactory {
	// If no filter config was provided, just return a function that does nothing.
	if v == nil {
		filterFunc := func(event *htevent.Event) bool {
			return true
		}
		return func() htfilter.Filter {
			return filterFunc
		}
	}

	rulesV := v.List()
	factories := make([]htfilter.FilterTLFactory, 0, rulesV.Len())

	for _, ruleRaw := range rulesV.All() {
		ruleV := ruleRaw.List()
		ruleParts := ruleV.All()
		if len(ruleParts) == 0 {
			ruleV.Fail("list must not be empty")
		}
		ruleType := ruleParts[0].String()
		ruleConfigureFunc, ok := ruleConfigureFuncs[ruleType]
		if !ok {
			ruleParts[0].Fail("unknown rule type %q", ruleType)
		}

		factory := ruleConfigureFunc(ruleV, ruleParts[1:])
		factories = append(factories, factory)
	}

	return func() htfilter.Filter {
		filters := make([]htfilter.Filter, len(factories))
		for i, factory := range factories {
			filters[i] = factory()
		}
		return func(event *htevent.Event) bool {
			for _, filter := range filters {
				keep := filter(event)
				if !keep {
					return false
				}
			}
			return true
		}
	}
}

var ruleConfigureFuncs map[string]htfilter.ConfigureFunc = map[string]htfilter.ConfigureFunc{
	"add":               ruleAdd,
	"set":               ruleSet,
	"drop":              ruleDrop,
	"sha256":            ruleSha256,
	"timestamp":         ruleTimestamp,
	"dynamic_sample":    htfilter_dynamicsample.Rule,
	"http_request_line": htfilter_httprequestline.Rule,
}

func ruleAdd(l sx.List, args []*sx.Value) htfilter.FilterTLFactory {
	if len(args) != 2 {
		l.Fail("expecting 2 arguments, got %d.", len(args))
	}
	fieldName := args[0].String()
	fieldValue := args[1].Any()
	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		_, present := event.Data[fieldName]
		if !present {
			event.Data[fieldName] = fieldValue
		}
		return true
	})
}

func ruleSet(l sx.List, args []*sx.Value) htfilter.FilterTLFactory {
	if len(args) != 2 {
		l.Fail("expecting 2 arguments, got %d.", len(args))
	}
	fieldName := args[0].String()
	fieldValue := args[1].Any()
	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		event.Data[fieldName] = fieldValue
		return true
	})
}

func ruleDrop(l sx.List, args []*sx.Value) htfilter.FilterTLFactory {
	if len(args) != 1 {
		l.Fail("expecting 1 argument, got %d.", len(args))
	}
	fieldName := args[0].String()
	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		delete(event.Data, fieldName)
		return true
	})
}

func ruleSha256(l sx.List, args []*sx.Value) htfilter.FilterTLFactory {
	if len(args) != 1 {
		l.Fail("expecting 1 argument, got %d.", len(args))
	}
	fieldName := args[0].String()
	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		v, present := event.Data[fieldName]
		if present {
			hash := sha256.Sum256([]byte(fmt.Sprintf("%v", v)))
			event.Data[fieldName] = fmt.Sprintf("%x", hash)
		}
		return true
	})
}

func ruleTimestamp(l sx.List, args []*sx.Value) htfilter.FilterTLFactory {
	if len(args) != 2 {
		l.Fail("expecting 2 arguments, got %d.", len(args))
	}
	fieldName := args[0].String()
	format := args[1].String()

	// TODO(kannan): Existing keyval/json parsers do a lot of magic with timestamps.
	// See how much of that magic we need to preserve.
	return htfilter.StatelessFactory(func(event *htevent.Event) bool {
		v, present := event.Data[fieldName]
		delete(event.Data, fieldName)

		if !present {
			logrus.Warnf("designated timestamp field %q is missing", fieldName)
			return true
		}

		s, isString := v.(string)
		if !isString {
			logrus.Warnf("designated timestamp field %q is not a string", fieldName)
			return true
		}

		ts, err := time.Parse(format, s)
		if err != nil {
			logrus.Warnf("designated timestamp field %q: couldn't parse %q with format %q",
				fieldName, s, format)
			return true
		}

		event.Timestamp = ts
		return true
	})
}
