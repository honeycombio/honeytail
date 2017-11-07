package postgresql

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/parsers"
	"github.com/honeycombio/mysqltools/query/normalizer"
)

var (
	timestamp = `^(?P<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} UTC)`
	connIDs   = `\s+\[(?P<pid>\d+)-(?P<session_id>\d+)\]`
	connInfo  = `\s+(?P<user>\S+)@(?P<database>\S+)`
	level     = `\s+(?P<level>[A-Z0-9]+):`
	duration  = `\s+duration: (?P<duration>[0-9\.]+) ms`
	statement = `\s+statement: `

	reLinePrefix = parsers.ExtRegexp{regexp.MustCompile(
		strings.Join([]string{timestamp, connIDs, connInfo, level, duration, statement}, ""),
	)}
)

type Parser struct{}

func (p *Parser) Init(options interface{}) error {
	return nil
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event, prefixRegex *parsers.ExtRegexp) {
	rawEvents := make(chan []string)
	go p.handleEvents(rawEvents, send)
	var groupedLines []string
	for line := range lines {
		if prefixRegex != nil {
			var prefix string
			prefix = prefixRegex.FindString(line)
			line = strings.TrimPrefix(line, prefix)
		}
		if !isContinuationLine(line) && len(groupedLines) > 0 {
			rawEvents <- groupedLines
			groupedLines = make([]string, 0, 1)
		}
		groupedLines = append(groupedLines, line)
	}

	rawEvents <- groupedLines
	close(rawEvents)
}

func (p *Parser) handleEvents(rawEvents <-chan []string, send chan<- event.Event) {
	// TODO: spin up a group of goroutines to do this
	for rawEvent := range rawEvents {
		ev := p.handleEvent(rawEvent)
		if ev != nil {
			send <- *ev
		}
	}
}

func (p *Parser) handleEvent(rawEvent []string) *event.Event {
	normalizer := normalizer.Parser{}
	if len(rawEvent) == 0 {
		return nil
	}
	firstLine := rawEvent[0]

	prefix, meta := reLinePrefix.FindStringSubmatchMap(firstLine)
	if prefix == "" {
		// Note: this may be noisy when debug logging is turned on, since the
		// postgres general log contains lots of other statements as well.
		logrus.WithField("line", firstLine).Debug("Log line didn't match expected format")
		return nil
	}

	data := make(map[string]interface{}, len(meta))

	sessionId, _ := strconv.Atoi(meta["session_id"])
	pid, _ := strconv.Atoi(meta["pid"])
	duration, _ := strconv.ParseFloat(meta["duration"], 64)

	data["pid"] = pid
	data["session_id"] = sessionId
	data["duration"] = duration
	data["user"] = meta["user"]
	data["database"] = meta["database"]

	query := firstLine[len(prefix):]
	for _, line := range rawEvent[1:] {
		query += " " + strings.TrimLeft(line, " \t")
	}
	normalizedQuery := normalizer.NormalizeQuery(query)

	data["query"] = query
	data["normalized_query"] = normalizedQuery
	if len(normalizer.LastTables) > 0 {
		data["tables"] = strings.Join(normalizer.LastTables, " ")
	}
	if len(normalizer.LastComments) > 0 {
		data["comments"] = "/* " + strings.Join(normalizer.LastComments, " */ /* ") + " */"
	}

	timestamp, err := time.Parse("2006-01-02 03:04:05 MST", meta["time"])
	if err != nil {
		logrus.WithError(err).WithField("time", meta["time"]).Debug("Error parsing timestamp")
		timestamp = time.Now()
	}

	return &event.Event{
		Data:      data,
		Timestamp: timestamp,
	}

}

func isContinuationLine(line string) bool {
	return strings.HasPrefix(line, "\t")
}
