package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/honeycombio/libhoney-go"
	flag "github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/honeycombio/honeytail/httime"
	"github.com/honeycombio/honeytail/parsers/arangodb"
	"github.com/honeycombio/honeytail/parsers/csv"
	"github.com/honeycombio/honeytail/parsers/htjson"
	"github.com/honeycombio/honeytail/parsers/keyval"
	"github.com/honeycombio/honeytail/parsers/mongodb"
	"github.com/honeycombio/honeytail/parsers/mysql"
	"github.com/honeycombio/honeytail/parsers/nginx"
	"github.com/honeycombio/honeytail/parsers/postgresql"
	"github.com/honeycombio/honeytail/parsers/regex"
	"github.com/honeycombio/honeytail/parsers/syslog"
	"github.com/honeycombio/honeytail/tail"
)

// BuildID is set by Travis CI
var BuildID string

// internal version identifier
var version string

var validParsers = []string{
	"arangodb",
	"csv",
	"json",
	"keyval",
	"mongo",
	"mysql",
	"nginx",
	"postgresql",
	"regex",
	"syslog",
}

// GlobalOptions has all the top level CLI flags that honeytail supports
type GlobalOptions struct {
	APIHost    string `long:"api_host" description:"Host for the Honeycomb API" default:"https://api.honeycomb.io/"`
	TailSample bool   `hidden:"true" description:"When true, sample while tailing. When false, sample post-parser events" yaml:"-"`

	ConfigFile string `short:"c" long:"config" description:"Config file for honeytail in INI format." no-ini:"true" yaml:"-"`
	ConfigYaml string `long:"config_yaml" description:"Config file for honeytail in YAML format." yaml:"-"`
	WriteYaml  string `long:"write_yaml" description:"When specified (a filename), parse the existing config, then write a new YAML config to the specified YAML file and quit." yaml:"-"`

	SampleRate       uint `short:"r" long:"samplerate" description:"Only send 1 / N log lines" default:"1" yaml:"samplerate"`
	NumSenders       uint `short:"P" long:"poolsize" description:"Number of concurrent connections to open to Honeycomb" default:"80" yaml:"poolsize"`
	BatchFrequencyMs uint `long:"send_frequency_ms" description:"How frequently to flush batches" default:"100" yaml:"send_frequency_ms"`
	BatchSize        uint `long:"send_batch_size" description:"Maximum number of messages to put in a batch" default:"50" yaml:"send_batch_size"`
	Debug            bool `long:"debug" description:"Print debugging output" yaml:"debug,omitempty"`
	DebugOut         bool `long:"debug_stdout" description:"Instead of sending events to Honeycomb, print them to STDOUT for debugging" yaml:"debug_stdout,omitempty"`
	StatusInterval   uint `long:"status_interval" description:"How frequently, in seconds, to print out summary info" default:"60" yaml:"status_interval"`
	Backfill         bool `long:"backfill" description:"Configure honeytail to ingest old data in order to backfill Honeycomb. Sets the correct values for --backoff, --tail.read_from, and --tail.stop" yaml:"backfill,omitempty"`
	RebaseTime       bool `long:"rebase_time" description:"When backfilling data, rebase timestamps relative to the current time." yaml:"rebase_time,omitempty"`

	Localtime           bool     `long:"localtime" description:"When parsing a timestamp that has no time zone, assume it is in the same timezone as localhost instead of UTC (the default)" yaml:"localtime,omitempty"`
	Timezone            string   `long:"timezone" description:"When parsing a timestamp use this time zone instead of UTC (the default). Must be specified in TZ format as seen here: https://en.wikipedia.org/wiki/List_of_tz_database_time_zones" yaml:"timezone,omitempty"`
	ScrubFields         []string `long:"scrub_field" description:"For the field listed, apply a one-way hash to the field content. May have multiple values." yaml:"scrub_field,omitempty"`
	DropFields          []string `long:"drop_field" description:"Do not send the field to Honeycomb. May have multiple values." yaml:"drop_field,omitempty"`
	AddFields           []string `long:"add_field" description:"Add the field to every event. Field should be key=val. May have multiple values." yaml:"add_field,omitempty"`
	DAMapFile           string   `long:"da_map_file" description:"Data Augmentation Map file. Path to a file that contains JSON mapping of columns to augment, the values of the column, and new objects to be inserted into the event, eg to add hostname based on IP address or username based on user ID." yaml:"da_map_file,omitempty"`
	RequestShape        []string `long:"request_shape" description:"Identify a field that contains an HTTP request of the form 'METHOD /path HTTP/1.x' or just the request path. Break apart that field into subfields that contain components. May have multiple values. Defaults to 'request' when using the nginx parser." yaml:"request_shape,omitempty"`
	ShapePrefix         string   `long:"shape_prefix" description:"Prefix to use on fields generated from request_shape to prevent field collision" yaml:"shape_prefix,omitempty"`
	RequestPattern      []string `long:"request_pattern" description:"A pattern for the request path on which to base the derived request_shape. May have multiple values. Patterns are considered in order; first match wins." yaml:"request_pattern,omitempty"`
	RequestParseQuery   string   `long:"request_parse_query" description:"How to parse the request query parameters. 'whitelist' means only extract listed query keys. 'all' means to extract all query parameters as individual columns" default:"whitelist" yaml:"request_parse_query"`
	RequestQueryKeys    []string `long:"request_query_keys" description:"Request query parameter key names to extract, when request_parse_query is 'whitelist'. May have multiple values." yaml:"request_query_keys,omitempty"`
	BackOff             bool     `long:"backoff" description:"When rate limited by the API, back off and retry sending failed events. Otherwise failed events are dropped. When --backfill is set, it will force this to true." yaml:"backoff,omitempty"`
	PrefixRegex         string   `long:"log_prefix" description:"pass a regex to this flag to strip the matching prefix from the line before handing to the parser. Useful when log aggregation prepends a line header. Use named groups to extract fields into the event." yaml:"log_prefix,omitempty"`
	DeterministicSample string   `long:"deterministic_sampling" description:"Specify a field to deterministically sample on, i.e., every concurrent Honeytail instance will sample 1/N based on content." yaml:"deterministic_sampling,omitempty"`
	DynSample           []string `long:"dynsampling" description:"Enable dynamic sampling using the field listed in this option. May have multiple values; fields will be concatenated to form the dynsample key. WARNING: increases CPU utilization dramatically over normal sampling." yaml:"dynsampling,omitempty"`
	DynWindowSec        int      `long:"dynsample_window" description:"Set measurement window size for the dynsampler, in seconds." default:"30" yaml:"dynsample_window"`
	PreSampledField     string   `long:"presampled" description:"If this log has already been sampled, specify the field containing the sample rate here and it will be passed along unchanged." yaml:"presampled,omitempty"`
	GoalSampleRate      int      `hidden:"true" description:"Used to hold the desired sample rate and set tailing sample rate to 1." yaml:"-"`
	MinSampleRate       int      `long:"dynsample_minimum" description:"If the rate of traffic falls below this, dynsampler won't sample." default:"1" yaml:"dynsample_minimum"`
	JSONFields          []string `long:"json_field" description:"JSON fields encoded as string to unescape and properly parse before sending. May have multiple values." yaml:"json_field,omitempty"`
	FilterFiles         []string `short:"F" long:"filter-file" description:"Log file(s) to exclude from --file glob. May have multiple values, including multiple globs." yaml:"filter-file,omitempty"`
	RenameFields        []string `long:"rename_field" description:"Format: 'before=after'. Rename field called 'before' from parsed lines to field name 'after' in Honeycomb events. May have multiple values." yaml:"rename_field,omitempty"`

	Reqs  RequiredOptions `group:"Required Options" yaml:"required_options,omitempty"`
	Modes OtherModes      `group:"Other Modes" yaml:"-"`

	Tail tail.TailOptions `group:"Tail Options" namespace:"tail" yaml:",omitempty"`

	ArangoDB   arangodb.Options   `group:"ArangoDB Parser Options" namespace:"arangodb" yaml:",omitempty"`
	CSV        csv.Options        `group:"CSV Parser Options" namespace:"csv" yaml:",omitempty"`
	JSON       htjson.Options     `group:"JSON Parser Options" namespace:"json" yaml:",omitempty"`
	KeyVal     keyval.Options     `group:"KeyVal Parser Options" namespace:"keyval" yaml:",omitempty"`
	Mongo      mongodb.Options    `group:"MongoDB Parser Options" namespace:"mongo" yaml:",omitempty"`
	MySQL      mysql.Options      `group:"MySQL Parser Options" namespace:"mysql" yaml:",omitempty"`
	Nginx      nginx.Options      `group:"Nginx Parser Options" namespace:"nginx" yaml:",omitempty"`
	PostgreSQL postgresql.Options `group:"PostgreSQL Parser Options" namespace:"postgresql" yaml:",omitempty"`
	Regex      regex.Options      `group:"Regex Parser Options" namespace:"regex" yaml:",omitempty"`
	Syslog     syslog.Options     `group:"Syslog Parser Options" namespace:"syslog" yaml:",omitempty"`
}

type RequiredOptions struct {
	ParserName string   `short:"p" long:"parser" description:"Parser module to use. Use --list to list available options."`
	WriteKey   string   `short:"k" long:"writekey" description:"Team write key"`
	LogFiles   []string `short:"f" long:"file" description:"Log file(s) to parse. Use '-' for STDIN, use this flag multiple times to tail multiple files, or use a glob (/path/to/foo-*.log)"`
	Dataset    string   `short:"d" long:"dataset" description:"Name of the dataset"`
}

type OtherModes struct {
	Help               bool `short:"h" long:"help" description:"Show this help message."`
	ListParsers        bool `short:"l" long:"list" description:"List available parsers."`
	Version            bool `short:"V" long:"version" description:"Show version."`
	WriteDefaultConfig bool `long:"write_default_config" description:"Write a default config file to STDOUT." no-ini:"true"`
	WriteCurrentConfig bool `long:"write_current_config" description:"Write out the current config to STDOUT." no-ini:"true"`
	WriteCurrentYaml   bool `long:"write_current_yaml" description:"Write out the current config to STDOUT as YAML." no-ini:"true"`

	WriteManPage bool `hidden:"true" long:"write-man-page" description:"Write out a man page."`
}

func main() {
	var options GlobalOptions
	flagParser := flag.NewParser(&options, flag.PrintErrors)
	flagParser.Usage = `-p <parser> -k <writekey> -f </path/to/logfile> -d <mydata> [optional arguments]

See https://honeycomb.io/docs/connect/agent/ for more detailed usage instructions.`

	if extraArgs, err := flagParser.Parse(); err != nil || len(extraArgs) != 0 {
		fmt.Println("Error: failed to parse the command line.")
		if err != nil {
			fmt.Printf("\t%s\n", err)
		} else {
			fmt.Printf("\tUnexpected extra arguments: %s\n", strings.Join(extraArgs, " "))
		}
		usage()
		os.Exit(1)
	}
	// read the config file if present
	if options.ConfigFile != "" {
		ini := flag.NewIniParser(flagParser)
		ini.ParseAsDefaults = true
		if err := ini.ParseFile(options.ConfigFile); err != nil {
			fmt.Printf("Error: failed to parse the config file %s\n", options.ConfigFile)
			fmt.Printf("\t%s\n", err)
			usage()
			os.Exit(1)
		}
	}

	if options.ConfigYaml != "" {
		f, err := os.Open(options.ConfigYaml)
		if err != nil {
			fmt.Println("error opening " + options.ConfigYaml)
			os.Exit(1)
		}
		b, err := io.ReadAll(f)
		if err != nil {
			fmt.Println("error reading " + options.ConfigYaml)
			os.Exit(1)
		}
		yaml.Unmarshal(b, &options)
	}

	rand.Seed(time.Now().UnixNano())

	if options.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Support flag alias: --backfill should cover --backoff --tail.read_from=beginning --tail.stop
	if options.Backfill {
		options.BackOff = true
		options.Tail.ReadFrom = "beginning"
		options.Tail.Stop = true
	}

	// set time zone info
	if options.Localtime {
		httime.Location = time.Now().Location()
	}
	if options.Timezone != "" {
		loc, err := time.LoadLocation(options.Timezone)
		if err != nil {
			fmt.Printf("time zone '%s' not successfully parsed.\n", options.Timezone)
			fmt.Printf("see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones for a list of time zones\n")
			fmt.Printf("expected format example: America/Los_Angeles\n")
			fmt.Printf("Specific error: %s\n", err.Error())
			os.Exit(1)
		}
		httime.Location = loc
	}

	setVersionUserAgent(options.Backfill, options.Reqs.ParserName)
	handleOtherModes(flagParser, options)
	addParserDefaultOptions(&options)
	sanityCheckOptions(&options)

	if !options.DebugOut {
		if _, err := libhoney.VerifyAPIKey(libhoney.Config{
			APIHost:  options.APIHost,
			WriteKey: options.Reqs.WriteKey,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Could not verify Honeycomb write key: ", err)
			os.Exit(1)
		}
	} else {
		logrus.Debug("skipping Honeycomb write key verification, because --debug_stdout is set...")
	}

	logrus.Debug("parsed arguments: ", structToString(options))

	run(context.Background(), options)
}

// convert options struct to a comma separated list of key=value pairs (for debugging)
func structToString(s interface{}) string {
	v := reflect.ValueOf(s)
	t := v.Type()
	fields := make([]string, v.NumField())

	isExported := func(s string) bool {
		f := s[0:1]
		return f == strings.ToUpper(f)
	}

	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Struct {
			fields[i] = structToString(v.Field(i).Interface())
		} else {
			name := t.Field(i).Name
			if isExported(name) {
				value := v.Field(i).Interface()
				if name == "WriteKey" {
					value = "[REDACTED]"
				}
				fields[i] = fmt.Sprintf("%s.%s=%v", t, name, value)
			}
		}
	}
	return strings.Join(fields, ",")
}

// setVersion sets the internal version ID and updates libhoney's user-agent
func setVersionUserAgent(backfill bool, parserName string) {
	if BuildID == "" {
		version = "dev"
	} else {
		version = BuildID
	}
	if backfill {
		parserName += " backfill"
	}
	libhoney.UserAgentAddition = fmt.Sprintf("honeytail/%s (%s)", version, parserName)
}

// handleOtherModes takes care of all flags that say we should just do something
// and exit rather than actually parsing logs
func handleOtherModes(fp *flag.Parser, options GlobalOptions) {
	modes := options.Modes
	if modes.Version {
		fmt.Println("Honeytail version", version)
		os.Exit(0)
	}
	if modes.Help {
		fp.WriteHelp(os.Stdout)
		fmt.Println("")
		os.Exit(0)
	}
	if modes.WriteManPage {
		fp.WriteManPage(os.Stdout)
		os.Exit(0)
	}
	if modes.WriteDefaultConfig {
		ip := flag.NewIniParser(fp)
		ip.Write(os.Stdout, flag.IniIncludeDefaults|flag.IniCommentDefaults|flag.IniIncludeComments)
		os.Exit(0)
	}
	if modes.WriteCurrentConfig {
		ip := flag.NewIniParser(fp)
		ip.Write(os.Stdout, flag.IniIncludeComments)
		os.Exit(0)
	}
	if modes.WriteCurrentYaml {
		y, err := yaml.Marshal(options)
		if err != nil {
			fmt.Println("unable to marshal options to YAML!")
			os.Exit(1)
		}
		os.Stdout.Write(y)
		os.Exit(0)
	}

	if modes.ListParsers {
		fmt.Println("Available parsers:", strings.Join(validParsers, ", "))
		os.Exit(0)
	}
}

func addParserDefaultOptions(options *GlobalOptions) {
	switch {
	case options.Reqs.ParserName == "nginx":
		// automatically normalize the request when using the nginx parser
		options.RequestShape = append(options.RequestShape, "request")
	}
	if options.Reqs.ParserName != "mysql" {
		// mysql is the only parser that requires in-parser sampling because it has
		// a multi-line log format.
		// Sample all other parser when tailing to conserve CPU
		options.TailSample = true
	} else {
		options.TailSample = false
	}
	if options.DeterministicSample != "" {
		options.TailSample = false
	}
	if len(options.DynSample) != 0 {
		// when using dynamic sampling, we make the sampling decision after parsing
		// the content, so we must not tailsample.
		options.TailSample = false
		options.GoalSampleRate = int(options.SampleRate)
		options.SampleRate = 1
	}
}

func sanityCheckOptions(options *GlobalOptions) {
	switch {
	case options.Reqs.ParserName == "":
		fmt.Println("Parser required to be specified with the --parser flag.")
		usage()
		os.Exit(1)
	case (options.Reqs.WriteKey == "" || options.Reqs.WriteKey == "NULL") && (!options.DebugOut):
		fmt.Println("Write key required to be specified with the --writekey flag.")
		usage()
		os.Exit(1)
	case len(options.Reqs.LogFiles) == 0:
		fmt.Println("Log file name or '-' required to be specified with the --file flag.")
		usage()
		os.Exit(1)
	case options.Reqs.Dataset == "":
		fmt.Println("Dataset name required with the --dataset flag.")
		usage()
		os.Exit(1)
	case options.SampleRate == 0:
		fmt.Println("Sample rate must be an integer >= 1")
		usage()
		os.Exit(1)
	case options.Tail.ReadFrom == "end" && options.Tail.Stop:
		fmt.Println("Reading from the end and stopping when we get there. Zero lines to process. Ok, all done! ;)")
		usage()
		os.Exit(1)
	case options.RequestParseQuery != "whitelist" && options.RequestParseQuery != "all":
		fmt.Println("request_parse_query flag must be either 'whitelist' or 'all'.")
		usage()
		os.Exit(1)
	case (len(options.DynSample) != 0 || options.DeterministicSample != "") && options.SampleRate <= 1 && options.GoalSampleRate <= 1:
		fmt.Println("sample rate flag must be set >= 2 when dynamic or deterministic sampling is enabled")
		usage()
		os.Exit(1)
	case len(options.DynSample) != 0 && options.DeterministicSample != "":
		fmt.Println("dynamic sampling and deterministic sampling cannot be used together")
		usage()
		os.Exit(1)
	case options.DeterministicSample != "" && options.PreSampledField != "":
		fmt.Println("deterministic sampling and a presampled field cannot be used together")
		usage()
		os.Exit(1)
	case options.SampleRate != 1 && options.PreSampledField != "":
		fmt.Println("sampling and a presampled field cannot be used together")
		usage()
		os.Exit(1)
	}

	// check the prefix regex for validity
	if options.PrefixRegex != "" {
		// make sure the regex is anchored against the start of the string
		if options.PrefixRegex[0] != '^' {
			options.PrefixRegex = "^" + options.PrefixRegex
		}
		// make sure it's valid
		_, err := regexp.Compile(options.PrefixRegex)
		if err != nil {
			fmt.Printf("Prefix regex %s doesn't compile: error %s\n", options.PrefixRegex, err)
			usage()
			os.Exit(1)
		}
	}

	// Make sure input files exist
	shouldExit := false
	for _, f := range options.Reqs.LogFiles {
		if f == "-" {
			continue
		}
		if files, err := filepath.Glob(f); err != nil || files == nil {
			fmt.Printf("Log file specified by --file=%s not found!\n", f)
			shouldExit = true
		}
	}
	if shouldExit {
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`
Usage: honeytail -p <parser> -k <writekey> -f </path/to/logfile> -d <mydata> [optional arguments]

For even more detail on required and optional parameters, run
honeytail --help
`)
}
