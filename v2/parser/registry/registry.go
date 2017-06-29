package registry

import (
	"fmt"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_line "github.com/honeycombio/honeytail/v2/parser/line"
	htparser_structured "github.com/honeycombio/honeytail/v2/parser/structured"

	htparser_nginx "github.com/honeycombio/honeytail/v2/parser/nginx"
    htparser_mysql "github.com/honeycombio/honeytail/v2/parser/mysql"
)

var configureFuncs map[string]htparser.ConfigureFunc = buildParserRegistry()

func buildParserRegistry() map[string]htparser.ConfigureFunc {

	all := map[string]htparser.ConfigureFunc{}

	// Parsers that implement the "structured" interface.
	structured := map[string]htparser_structured.ConfigureFunc{
		"mysql": htparser_mysql.Configure,
	}
	for name, f := range structured {
		if _, present := all[name]; present {
			panic(fmt.Sprintf("duplicate parser name %q", name))
		}
		all[name] = htparser_structured.ToStandardConfigureFunc(f)
	}

	// Parsers that implement the "line" interface.
	line := map[string]htparser_line.ConfigureFunc{
		"nginx": htparser_nginx.Configure,
	}
	for name, f := range line {
		if _, present := all[name]; present {
			panic(fmt.Sprintf("duplicate parser name %q", name))
		}
		all[name] = htparser_structured.ToStandardConfigureFunc(
			htparser_line.ToStructuredConfigureFunc(f))
	}

	return all
}

func Configure(v *sx.Value) htparser.SetupFunc {
	engineType, engineConfig := v.TaggedUnion()
	configureFunc, ok := configureFuncs[engineType]
	if !ok {
		v.Fail("unknown type %q", engineType)
	}

	return configureFunc(engineConfig)
}
