package registry

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

    htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_nginx "github.com/honeycombio/honeytail/v2/parser/nginx"
    htparser_mysql "github.com/honeycombio/honeytail/v2/parser/mysql"
)

var builders map[string]htparser.BuildFunc = map[string]htparser.BuildFunc{
    "nginx": htparser_nginx.Build,
	"mysql": htparser_mysql.Build,
}

func Build(v *sx.Value) htparser.StartFunc {
	sourceType, sourceConfig := v.TaggedUnion()
	buildFunc, ok := builders[sourceType]
	if !ok {
		v.Fail("unknown source type %q", sourceType)
	}

	return buildFunc(sourceConfig)
}
