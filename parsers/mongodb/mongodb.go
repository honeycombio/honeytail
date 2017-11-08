// Package mongodb is a parser for mongodb logs
package mongodb

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/mongodbtools/logparser"
	queryshape "github.com/honeycombio/mongodbtools/queryshape"

	"github.com/honeycombio/honeytail/event"
	"github.com/honeycombio/honeytail/httime"
	"github.com/honeycombio/honeytail/parsers"
	"github.com/honeycombio/honeytail/reporting"
)

const (
	// https://github.com/rueckstiess/mongodb-log-spec#timestamps
	ctimeNoMSTimeFormat    = "Mon Jan _2 15:04:05"
	ctimeTimeFormat        = "Mon Jan _2 15:04:05.000"
	iso8601UTCTimeFormat   = "2006-01-02T15:04:05.000Z"
	iso8601LocalTimeFormat = "2006-01-02T15:04:05.000-0700"

	timestampFieldName   = "timestamp"
	namespaceFieldName   = "namespace"
	databaseFieldName    = "database"
	collectionFieldName  = "collection"
	locksFieldName       = "locks"
	locksMicrosFieldName = "locks(micros)"

	shardingChangelogFieldName   = "sharding_changelog"
	changelogWhatFieldName       = "changelog_what"
	changelogChangeIDFieldName   = "changelog_changeid"
	changelogPrimaryFieldName    = "changelog_primary"
	changelogServerFieldName     = "changelog_server"
	changelogClientAddrFieldName = "changelog_client_addr"
	changelogTimeFieldName       = "changelog_time"
	changelogDetailsFieldName    = "changelog_details"
)

var timestampFormats = []string{
	iso8601LocalTimeFormat,
	iso8601UTCTimeFormat,
	ctimeTimeFormat,
	ctimeNoMSTimeFormat,
}

type Options struct {
	LogPartials bool `long:"log_partials" description:"Send what was successfully parsed from a line (only if the error occured in the log line's message)." json:",omitempty"`

	NumParsers int `hidden:"true" description:"number of mongo parsers to spin up" json:",omitempty"`
}

type Parser struct {
	conf Options

	lock              sync.RWMutex
	currentReplicaSet string
}

type MongoLineParser struct {
}

func (m *MongoLineParser) ParseLine(line string) (map[string]interface{}, error) {
	return logparser.ParseLogLine(line)
}

func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)
	return nil
}

func (p *Parser) ProcessLines(ctx context.Context, lines <-chan string, send chan<- event.Event, prefixRegex *parsers.ExtRegexp) {
	wg := sync.WaitGroup{}
	numParsers := 1
	if p.conf.NumParsers > 0 {
		numParsers = p.conf.NumParsers
	}
	for i := 0; i < numParsers; i++ {
		wg.Add(1)
		go func() {
			lineParser := &MongoLineParser{}
			for line := range lines {
				line = strings.TrimSpace(line)
				// take care of any headers on the line
				var prefixFields map[string]string
				if prefixRegex != nil {
					var prefix string
					prefix, prefixFields = prefixRegex.FindStringSubmatchMap(line)
					line = strings.TrimPrefix(line, prefix)
				}
				values, err := lineParser.ParseLine(line)
				// we get a bunch of errors from the parser on mongo logs, skip em
				if err == nil || (p.conf.LogPartials && logparser.IsPartialLogLine(err)) {
					timestamp, err := p.parseTimestamp(values)
					if err != nil {
						reporting.ParseError(ctx, line, err)
						continue
					}
					p.decomposeSharding(values)
					p.decomposeNamespace(values)
					p.decomposeLocks(values)
					p.decomposeLocksMicros(values)
					p.getCommandQuery(values)

					if q, ok := values["query"].(map[string]interface{}); ok {
						if _, ok = values["normalized_query"]; !ok {
							// also calculate the query_shape if we can
							values["normalized_query"] = queryshape.GetQueryShape(q)
						}
					}

					if ns, ok := values["namespace"].(string); ok && ns == "admin.$cmd" {
						if cmdType, ok := values["command_type"]; ok && cmdType == "replSetHeartbeat" {
							if cmd, ok := values["command"].(map[string]interface{}); ok {
								if replicaSet, ok := cmd["replSetHeartbeat"].(string); ok {
									p.lock.Lock()
									p.currentReplicaSet = replicaSet
									p.lock.Unlock()
								}
							}
						}
					}

					p.lock.RLock()
					if p.currentReplicaSet != "" {
						values["replica_set"] = p.currentReplicaSet
					}
					p.lock.RUnlock()

					// merge the prefix fields and the parsed line contents
					for k, v := range prefixFields {
						values[k] = v
					}

					logrus.WithFields(logrus.Fields{
						"line":      line,
						"values":    values,
						"timestamp": timestamp,
					}).Debug("Success: parsed line")

					// we'll be putting the timestamp in the Event
					// itself, no need to also have it in the Data
					delete(values, timestampFieldName)

					send <- event.Event{
						Timestamp: timestamp,
						Data:      values,
					}
				} else {
					reporting.ParseError(ctx, line, err)
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	logrus.Debug("lines channel is closed, ending mongo processor")
}

func (p *Parser) parseTimestamp(values map[string]interface{}) (time.Time, error) {
	now := httime.Now()
	timestamp_value, ok := values[timestampFieldName].(string)
	if ok {
		var err error
		for _, f := range timestampFormats {
			var timestamp time.Time
			timestamp, err = httime.Parse(f, timestamp_value)
			if err == nil {
				if f == ctimeTimeFormat || f == ctimeNoMSTimeFormat {
					// these formats lacks the year, so we check
					// if adding Now().Year causes the date to be
					// after today.  if it's after today, we
					// decrement year by 1.  if it's not after, we
					// use it.
					ts := timestamp.AddDate(now.Year(), 0, 0)
					if now.After(ts) {
						return ts, nil
					}

					return timestamp.AddDate(now.Year()-1, 0, 0), nil
				}
				return timestamp, nil
			}
		}
		return time.Time{}, err
	}

	return time.Time{}, errors.New("timestamp missing from logline")
}

func (p *Parser) decomposeSharding(values map[string]interface{}) {
	clValue, ok := values[shardingChangelogFieldName]
	if !ok {
		return
	}
	clMap, ok := clValue.(map[string]interface{})
	if !ok {
		return
	}

	var val interface{}
	if val, ok = clMap["ns"]; ok {
		values[namespaceFieldName] = val
	}
	if val, ok = clMap["_id"]; ok {
		values[changelogChangeIDFieldName] = val
	}
	if val, ok = clMap["server"]; ok {
		values[changelogServerFieldName] = val
	}
	if val, ok = clMap["clientAddr"]; ok {
		values[changelogClientAddrFieldName] = val
	}
	if val, ok = clMap["time"]; ok {
		values[changelogTimeFieldName] = val
	}
	if val, ok = clMap["what"]; ok {
		values[changelogWhatFieldName] = val
	}
	detailsMap, ok := clMap["details"].(map[string]interface{})
	if ok {
		values[changelogDetailsFieldName] = detailsMap
		values[changelogPrimaryFieldName] = detailsMap["primary"]
	}

	delete(values, shardingChangelogFieldName)
}

func (p *Parser) decomposeNamespace(values map[string]interface{}) {
	ns_value, ok := values[namespaceFieldName]
	if !ok {
		return
	}

	decomposed := strings.SplitN(ns_value.(string), ".", 2)
	if len(decomposed) < 2 {
		return
	}
	values[databaseFieldName] = decomposed[0]
	values[collectionFieldName] = decomposed[1]
}

func (p *Parser) decomposeLocks(values map[string]interface{}) {
	locks_value, ok := values[locksFieldName]
	if !ok {
		return
	}
	locks_map, ok := locks_value.(map[string]interface{})
	if !ok {
		return
	}
	for scope, v := range locks_map {
		v_map, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		for attrKey, attrVal := range v_map {
			attrVal_map, ok := attrVal.(map[string]interface{})
			if !ok {
				continue
			}
			for lockType, lockCount := range attrVal_map {
				if lockType == "r" {
					lockType = "read"
				} else if lockType == "R" {
					lockType = "Read"
				} else if lockType == "w" {
					lockType = "write"
				} else if lockType == "w" {
					lockType = "Write"
				}

				if attrKey == "acquireCount" {
					values[strings.ToLower(scope)+"_"+lockType+"_lock"] = lockCount
				} else if attrKey == "acquireWaitCount" {
					values[strings.ToLower(scope)+"_"+lockType+"_lock_wait"] = lockCount
				} else if attrKey == "timeAcquiringMicros" {
					values[strings.ToLower(scope)+"_"+lockType+"_lock_wait_us"] = lockCount
				}
			}
		}
	}
	delete(values, locksFieldName)
	return
}

func (p *Parser) decomposeLocksMicros(values map[string]interface{}) {
	locks_value, ok := values[locksMicrosFieldName]
	if !ok {
		return
	}
	locks_map, ok := locks_value.(map[string]int64)
	if !ok {
		return
	}
	for lockType, lockDuration := range locks_map {
		if lockType == "r" {
			lockType = "read"
		} else if lockType == "R" {
			lockType = "Read"
		} else if lockType == "w" {
			lockType = "write"
		} else if lockType == "w" {
			lockType = "Write"
		}

		values[lockType+"_lock_held_us"] = lockDuration
	}
	delete(values, locksMicrosFieldName)
}

func (p *Parser) getCommandQuery(values map[string]interface{}) {
	if commandType, ok := values["command_type"]; ok {
		if cmd, ok := values["command"].(map[string]interface{}); ok {
			switch commandType {
			case "find":
				q, ok := cmd["filter"].(map[string]interface{})
				if ok {
					// skip the $where queries, since those are
					// strings with embedded javascript expressions
					if _, ok = q["$where"]; !ok {
						values["query"] = q
					}
				}
				break
			case "findAndModify":
				q, ok := cmd["query"]
				if ok {
					values["query"] = q
				}
				break
			case "update":
				// update is special in that each update log can contain multiple update statements.
				// we build up a synthetic query that includes the entirety of the update list (with
				// modifications so that the normalizer will include more info.)
				updates, ok := cmd["updates"].([]interface{})
				if ok {
					fakeQuery := make(map[string]interface{})
					var newUpdates []interface{}
					for _, _update := range updates {
						update, ok := _update.(map[string]interface{})
						if !ok {
							continue
						}

						newU := make(map[string]interface{})
						if q, ok := update["q"]; ok {
							newU["$query"] = q
						}
						if u, ok := update["u"]; ok {
							newU["$update"] = u
						}
						if setOnInsert, ok := update["$setOnInsert"]; ok {
							newU["$setOnInsert"] = setOnInsert
						}

						newUpdates = append(newUpdates, newU)
					}
					fakeQuery["updates"] = newUpdates
					values["query"] = fakeQuery
				}
				break
			case "delete":
				// same treatment as with update above
				deletes, ok := cmd["deletes"].([]interface{})
				if ok {
					fakeQuery := make(map[string]interface{})
					var newDeletes []interface{}
					for _, _del := range deletes {
						del, ok := _del.(map[string]interface{})
						if !ok {
							continue
						}

						newD := make(map[string]interface{})
						if q, ok := del["q"]; ok {
							newD["$query"] = q
						}
						if lim, ok := del["limit"]; ok {
							newD["$limit"] = lim
						}

						newDeletes = append(newDeletes, newD)
					}
					fakeQuery["deletes"] = newDeletes
					values["query"] = fakeQuery
				}
				break
			}

		}
	}
}
