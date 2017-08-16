package types

import flags "github.com/jessevdk/go-flags"

func FilenameFlagFromString(str string) flags.Filename {
	return flags.Filename(str)
}
