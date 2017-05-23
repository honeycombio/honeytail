package systemd

import (
	"bufio"
	"os/exec"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/parsers"
	"github.com/honeycombio/honeytail/parsers/htjson"
)

// systemd will output log lines in JSON when passed certain options, so the
// systemd parser embeds the htjson parser
type Parser struct {
	*htjson.Parser
}

func (p *Parser) Init(options interface{}) error {
	p.Parser = &htjson.Parser{}
	return p.Parser.Init(options)
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event, prefixRegex *parsers.ExtRegexp) {
	// normal lines doesn't get passed to the method since we are not
	// reading from files on disk in this driver
	//
	// TODO(sandalwing): Work on defining how this fits into the
	// general honeytail model.
	//
	// Worth noting that not all logs come from disk. In fact they make
	// come from a variety of sources such as logstash.
	journalctlLines := make(chan string)
	go p.followJournal(journalctlLines)
	p.Parser.ProcessLines(journalctlLines, send, prefixRegex)
}

func (p *Parser) followJournal(lines chan string) {
	cmd := exec.Command("journalctl", "-f", "-o", "json")
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		// skip lines that won't parse
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("journalctl pipe failed")
	}

	if err != nil {
		// skip lines that won't parse
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("making journalctl scanner failed")
	}
	scanner := bufio.NewScanner(cmdReader)

	go cmd.Start()

	for scanner.Scan() {
		lines <- scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("journalctl bailed!! :(")
	}
}
