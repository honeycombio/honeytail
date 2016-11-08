// Package tail implements tailing a log file.
//
// tail provides a channel on which log lines will be sent as string messages.
// one line in the log file is one message on the channel
package tail

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/hpcloud/tail"
	"golang.org/x/sys/unix"
)

type RotateStyle int

const (
	// foo.log gets rotated to foo.log.1, new entries go to foo.log
	RotateStyleSyslog RotateStyle = iota
	// foo.log.OLDSTAMP gets closed, new entries go to foo.log.NEWSTAMP
	// NOT YET IMPLEMENTED
	RotateStyleTimestamp
)

type TailOptions struct {
	ReadFrom  string `long:"read_from" description:"Location in the file from which to start reading. Values: beginning, end, last. Last picks up where it left off, if the file has not been rotated, otherwise beginning. When --backfill is set, it will override this option=beginning" default:"last"`
	Stop      bool   `long:"stop" description:"Stop reading the file after reaching the end rather than continuing to tail. When --backfill is set, it will override this option=true"`
	Poll      bool   `long:"poll" description:"use poll instead of inotify to tail files"`
	StateFile string `long:"statefile" description:"File in which to store the last read position. Defaults to a file with the same path as the log file and the suffix .leash.state. If tailing multiple files, default is forced."`
}

// Statefile mechanics when ReadFrom is 'last'
// missing statefile => ReadFrom = end
// empty statefile => ReadFrom = end
// permission denied => WARN and ReadFrom = end
// invalid location (aka logfile's been rotated) => ReadFrom = beginning

type Config struct {
	// Path to the log file to tail
	Paths []string
	// Type of log rotation we expect on this file
	Type RotateStyle
	// Tail specific options
	Options TailOptions
}

// State is what's stored in a statefile
type State struct {
	LogfileName   string
	StatefileName string
	INode         uint64 // the inode
	Offset        int64
	Stat          unix.Stat_t `json:"-"`
	TrailingBy    int64       `json:"-"`
}

type Tailer struct {
	Lines        chan string
	state        State
	linesRead    int
	linesErrored int
	statsLock    *sync.Mutex
}

func (t *Tailer) LogStats() {
	logrus.WithFields(logrus.Fields{
		"logfile":       t.state.LogfileName,
		"inode":         t.state.INode,
		"offset":        t.state.Offset,
		"length":        t.state.Stat.Size,
		"lines_read":    t.linesRead,
		"lines_errored": t.linesErrored,
		"bytes_behind":  t.state.TrailingBy,
	}).Info("Logfile Tail Status")
	t.resetStats()
}

func (t *Tailer) resetStats() {
	t.statsLock.Lock()
	defer t.statsLock.Unlock()
	t.linesRead = 0
	t.linesErrored = 0
}

// GetEntries sets up a list of channels that get one line at a time from each
// file down each channel.
func GetEntries(conf Config) ([]*Tailer, error) {
	if conf.Type != RotateStyleSyslog {
		return nil, errors.New("Only Syslog style rotation currently supported")
	}
	// expand any globs in the list of files so our list all represents real files
	var filenames []string
	for _, filePath := range conf.Paths {
		if filePath == "-" {
			filenames = append(filenames, filePath)
		} else {
			files, err := filepath.Glob(filePath)
			if err != nil {
				return nil, err
			}
			files = removeStateFiles(files, conf)
			filenames = append(filenames, files...)
		}
	}
	if len(filenames) > 1 {
		// when tailing multiple files, force the default statefile use
		conf.Options.StateFile = ""
	}
	if len(filenames) == 0 {
		return nil, errors.New("After removing missing files and state files from the list, there are no files left to tail")
	}

	// make our lines channel list; we'll get one channel for each file
	tailers := make([]*Tailer, 0, len(filenames))
	for _, file := range filenames {
		tailer := &Tailer{
			statsLock: &sync.Mutex{},
			state:     State{},
		}
		var lines chan string
		if file == "-" {
			tailer.state.LogfileName = "STDIN"
			lines = tailStdIn()
		} else {
			tailer.state.LogfileName = file
			stateFile := getStateFile(conf, file)
			tailer.state.StatefileName = stateFile
			fileTailer, err := tailer.getTailer(conf)
			if err != nil {
				return nil, err
			}
			lines = tailer.tailSingleFile(fileTailer)
		}
		tailer.Lines = lines
		tailers = append(tailers, tailer)
	}

	return tailers, nil
}

// removeStateFiles goes through the list of files and removes any that appear
// to be statefiles to avoid .leash.state.leash.state.leash.state from appearing
// when you use an overly permissive glob
func removeStateFiles(files []string, conf Config) []string {
	newFiles := []string{}
	for _, file := range files {
		if file == conf.Options.StateFile {
			logrus.WithFields(logrus.Fields{
				"file": file,
			}).Debug("skipping tailing file because it is named the same as the statefile flag")
			continue
		}
		if strings.HasSuffix(file, ".leash.state") {
			logrus.WithFields(logrus.Fields{
				"file": file,
			}).Debug("skipping tailing file because the filename ends with .leash.state")
			continue
		}
		// great! it's not a state file. let's use it.
		newFiles = append(newFiles, file)
	}
	return newFiles
}

// func tailSingleFile(tailer *tail.Tail, file string, stateFile string) chan string {
func (t *Tailer) tailSingleFile(fileTailer *tail.Tail) chan string {
	lines := make(chan string)
	// TODO report some metric to indicate whether we're keeping up with the
	// front of the file, of if it's being written faster than we can send
	// events

	// initializet the tailer stats
	t.updateStats(fileTailer)
	// TODO this only updates once/sec. On clean shutdown, make sure we write
	// one last time after stopping reading traffic.
	go t.updateStateFile(fileTailer)

	go func() {
		for line := range fileTailer.Lines {
			t.linesRead++
			if line.Err != nil {
				// count and skip errored lines
				t.linesErrored++
				continue
			}
			lines <- strings.TrimSpace(line.Text)
		}
		close(lines)
	}()
	return lines
}

// tailStdIn is a special case to tail STDIN without any of the
// fancy stuff that the tail module provides
func tailStdIn() chan string {
	lines := make(chan string)
	input := bufio.NewReader(os.Stdin)
	go func() {
		defer close(lines)
		for {
			line, partialLine, err := input.ReadLine()
			if err != nil {
				logrus.Debug("stdin is closed")
				// bail when STDIN closes
				return
			}
			var parts []string
			parts = append(parts, string(line))
			for partialLine {
				line, partialLine, _ = input.ReadLine()
				parts = append(parts, string(line))
			}
			lines <- strings.Join(parts, "")
		}
	}()
	return lines
}

// getStartLocation reads the state file and creates an appropriate start
// location.  See details at the top of this file on how the loc is chosen.
func getStartLocation(stateFile string, logfile string) *tail.SeekInfo {
	beginning := &tail.SeekInfo{}
	end := &tail.SeekInfo{0, 2}
	fh, err := os.Open(stateFile)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"starting at": "end", "error": err,
		}).Debug("getStartLocation failed to open the statefile")
		return end
	}
	defer fh.Close()
	// read the contents of the state file (JSON)
	content := make([]byte, 1024)
	bytesRead, err := fh.Read(content)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"starting at": "end", "error": err,
		}).Debug("getStartLocation failed to read the statefile contents")
		return end
	}
	content = content[:bytesRead]
	// decode the contents of the statefile
	state := State{}
	if err := json.Unmarshal(content, &state); err != nil {
		logrus.WithFields(logrus.Fields{
			"starting at": "end", "error": err,
		}).Debug("getStartLocation failed to json decode the statefile")
		return end
	}
	// get the details of the existing log file
	logStat := unix.Stat_t{}
	if err := unix.Stat(logfile, &logStat); err != nil {
		logrus.WithFields(logrus.Fields{
			"starting at": "end", "error": err,
		}).Debug("getStartLocation failed to get unix.stat() on the logfile")
		return end
	}
	// compare inode numbers of the last-seen and existing log files
	if state.INode != logStat.Ino {
		logrus.WithFields(logrus.Fields{
			"starting at": "beginning", "error": err,
		}).Debug("getStartLocation found a different inode number for the logfile")
		// file's been rotated
		return beginning
	}
	logrus.WithFields(logrus.Fields{
		"starting at": state.Offset,
	}).Debug("getStartLocation seeking to offset in logfile")
	// we're good; start reading from the remembered state
	return &tail.SeekInfo{
		Offset: state.Offset,
		Whence: 0,
	}
}

// getTailer configures the *tail.Tail correctly to begin actually tailing the
// specified file.
// func getTailer(conf Config, file string, stateFile string) (*tail.Tail, error) {
func (t *Tailer) getTailer(conf Config) (*tail.Tail, error) {
	// tail a real file
	var loc *tail.SeekInfo // 0 value means start at beginning
	var reOpen, follow bool = true, true
	switch conf.Options.ReadFrom {
	case "start", "beginning":
		// 0 value for tail.SeekInfo means start at beginning
	case "end":
		loc = &tail.SeekInfo{
			Offset: 0,
			Whence: 2,
		}
	case "last":
		loc = getStartLocation(t.state.StatefileName, t.state.LogfileName)
	default:
		errMsg := fmt.Sprintf("unknown option to --read_from: %s",
			conf.Options.ReadFrom)
		return nil, errors.New(errMsg)
	}
	if conf.Options.Stop {
		reOpen = false
		follow = false
	}
	tailConf := tail.Config{
		Location:  loc,
		ReOpen:    reOpen, // keep reading on rotation, aka tail -F
		MustExist: true,   // fail if log file doesn't exist
		Follow:    follow, // don't stop at EOF, aka tail -f
		Logger:    tail.DiscardingLogger,
		Poll:      conf.Options.Poll, // use poll instead of inotify
	}
	logrus.WithFields(logrus.Fields{
		"tailConf":  tailConf,
		"conf":      conf,
		"statefile": t.state.StatefileName,
		"location":  loc,
	}).Debug("about to call tail.TailFile")
	return tail.TailFile(t.state.LogfileName, tailConf)
}

// getStateFile returns the filename to use to track honeytail state. If
// provided in the Config, uses the provided value instead.
func getStateFile(conf Config, filename string) string {
	if conf.Options.StateFile != "" {
		return conf.Options.StateFile
	}
	return strings.TrimSuffix(filename, ".log") + ".leash.state"
}

// updateStateFile updates the state file once per second with the current
// values for the logfile's inode number and offset
func (t *Tailer) updateStateFile(fileTailer *tail.Tail) {
	statefh, err := os.OpenFile(t.state.StatefileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"logfile":   t.state.LogfileName,
			"statefile": t.state.StatefileName,
		}).Warn("Failed to open statefile for writing. File location will not be saved.")
		return
	}
	ticker := time.NewTicker(time.Second)
	for _ = range ticker.C {
		if err := t.updateStats(fileTailer); err != nil {
			continue
		}
		out, err := json.Marshal(t.state)
		if err != nil {
			continue
		}
		statefh.Truncate(0)
		out = append(out, '\n')
		statefh.WriteAt(out, 0)
		statefh.Sync()
	}
}

func (t *Tailer) updateStats(fileTailer *tail.Tail) error {
	logStat := unix.Stat_t{}
	unix.Stat(t.state.LogfileName, &logStat)
	t.state.Stat = logStat
	t.state.INode = logStat.Ino
	if fileTailer == nil {
		return nil
	}
	currentPos, err := fileTailer.Tell()
	if err != nil {
		return err
	}
	t.state.Offset = currentPos
	t.state.TrailingBy = t.state.Stat.Size - currentPos
	return nil
}
