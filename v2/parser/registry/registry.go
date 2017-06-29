package registry

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

    htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_nginx "github.com/honeycombio/honeytail/v2/parser/nginx"
    htparser_mysql "github.com/honeycombio/honeytail/v2/parser/mysql"
)

var builders map[string]htparser.ConfigureFunc = map[string]htparser.ConfigureFunc{
    "nginx": htparser_nginx.Configure,
	"mysql": htparser_mysql.Configure,
}

func Configure(v *sx.Value) htparser.SetupFunc {
	sourceType, sourceConfig := v.TaggedUnion()
	configureFunc, ok := builders[sourceType]
	if !ok {
		v.Fail("unknown parser type %q", sourceType)
	}

	return configureFunc(sourceConfig)
}
