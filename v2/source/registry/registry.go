package registry

import (
	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htsource "github.com/honeycombio/honeytail/v2/source"
	htsource_files "github.com/honeycombio/honeytail/v2/source/files"
)

var builders map[string]htsource.BuildFunc = map[string]htsource.BuildFunc{
	"files": htsource_files.Build,
}

func Build(v *sx.Value, backfill bool) htsource.StartFunc {
	sourceType, sourceConfig := v.TaggedUnion()
	buildFunc, ok := builders[sourceType]
	if !ok {
		v.Fail("unknown source type %q", sourceType)
	}

	return buildFunc(sourceConfig, backfill)
}
