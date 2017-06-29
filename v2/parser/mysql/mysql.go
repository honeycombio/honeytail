// Package mysql parses the mysql slow query log
package mysql

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"github.com/Sirupsen/logrus"
	_ "github.com/go-sql-driver/mysql"
	"github.com/honeycombio/mysqltools/query/normalizer"

	sx "github.com/honeycombio/honeytail/v2/struct_extractor"

	htparser "github.com/honeycombio/honeytail/v2/parser"
	htparser_structured "github.com/honeycombio/honeytail/v2/parser/structured"
	htutil "github.com/honeycombio/honeytail/v2/util"
)

// See mysql_test for example log entries
// 3 sample log entries
//
// # Time: 2016-04-01T00:31:09.817887Z
// # User@Host: root[root] @ localhost []  Id:   233
// # Query_time: 0.008393  Lock_time: 0.000154 Rows_sent: 1  Rows_examined: 357
// SET timestamp=1459470669;
// show status like 'Uptime';
//
// # Time: 2016-04-01T00:31:09.853523Z
// # User@Host: root[root] @ localhost []  Id:   233
// # Query_time: 0.020424  Lock_time: 0.000147 Rows_sent: 494  Rows_examined: 494
// SET timestamp=1459470669;
// SHOW /*innotop*/ GLOBAL VARIABLES;
//
// # Time: 2016-04-01T00:31:09.856726Z
// # User@Host: root[root] @ localhost []  Id:   233
// # Query_time: 0.000021  Lock_time: 0.000000 Rows_sent: 494  Rows_examined: 494
// SET timestamp=1459470669;
// # administrator command: Ping;
//
// We should ignore the administrator command entry; the stats it presents (eg rows_sent)
// are actually for the previous command
//
// Sample line from RDS
// # administrator command: Prepare;
// # User@Host: root[root] @  [10.0.1.76]  Id: 325920
// # Query_time: 0.000097  Lock_time: 0.000023 Rows_sent: 1  Rows_examined: 1
// SET timestamp=1476127288;
// SELECT * FROM foo WHERE bar=2 AND (baz=104 OR baz=0) ORDER BY baz;

const (
	// Event attributes
	userKey            = "user"
	clientKey          = "client"
	queryTimeKey       = "query_time"
	lockTimeKey        = "lock_time"
	rowsSentKey        = "rows_sent"
	rowsExaminedKey    = "rows_examined"
	rowsAffectedKey    = "rows_affected"
	databaseKey        = "database"
	queryKey           = "query"
	normalizedQueryKey = "normalized_query"
	statementKey       = "statement"
	tablesKey          = "tables"
	commentsKey        = "comments"
	// InnoDB keys (it seems)
	bytesSentKey      = "bytes_sent"
	tmpTablesKey      = "tmp_tables"
	tmpDiskTablesKey  = "tmp_disk_tables"
	tmpTableSizesKey  = "tmp_table_sizes"
	transactionIDKey  = "transaction_id"
	queryCacheHitKey  = "query_cache_hit"
	fullScanKey       = "full_scan"
	fullJoinKey       = "full_join"
	tmpTableKey       = "tmp_table"
	tmpTableOnDiskKey = "tmp_table_on_disk"
	fileSortKey       = "filesort"
	fileSortOnDiskKey = "filesort_on_disk"
	mergePassesKey    = "merge_passes"
	ioROpsKey         = "IO_r_ops"
	ioRBytesKey       = "IO_r_bytes"
	ioRWaitKey        = "IO_r_wait_sec"
	recLockWaitKey    = "rec_lock_wait_sec"
	queueWaitKey      = "queue_wait_sec"
	pagesDistinctKey  = "pages_distinct"
)

var (
	reTime             = htutil.ExtRegexp{regexp.MustCompile("^# Time: (?P<time>[^ ]+)Z *$")}
	reAdminPing        = htutil.ExtRegexp{regexp.MustCompile("^# administrator command: Ping; *$")}
	reUser             = htutil.ExtRegexp{regexp.MustCompile("^# User@Host: (?P<user>[^#]+) @ (?P<host>[^#]+?)( Id:.+)?$")}
	reQueryStats       = htutil.ExtRegexp{regexp.MustCompile("^# Query_time: (?P<queryTime>[0-9.]+) *Lock_time: (?P<lockTime>[0-9.]+) *Rows_sent: (?P<rowsSent>[0-9]+) *Rows_examined: (?P<rowsExamined>[0-9]+)( *Rows_affected: (?P<rowsAffected>[0-9]+))?.*$")}
	reServStats        = htutil.ExtRegexp{regexp.MustCompile("^# Bytes_sent: (?P<bytesSent>[0-9.]+) *Tmp_tables: (?P<tmpTables>[0-9.]+) *Tmp_disk_tables: (?P<tmpDiskTables>[0-9]+) *Tmp_table_sizes: (?P<tmpTableSizes>[0-9]+).*$")}
	reInnodbTrx        = htutil.ExtRegexp{regexp.MustCompile("^# InnoDB_trx_id: (?P<trxId>[A-F0-9]+) *$")}
	reInnodbQueryPlan1 = htutil.ExtRegexp{regexp.MustCompile("^# QC_Hit: (?P<query_cache_hit>[[:alpha:]]+)  Full_scan: (?P<full_scan>[[:alpha:]]+)  Full_join: (?P<full_join>[[:alpha:]]+)  Tmp_table: (?P<tmp_table>[[:alpha:]]+)  Tmp_table_on_disk: (?P<tmp_table_on_disk>[[:alpha:]]+).*$")}
	reInnodbQueryPlan2 = htutil.ExtRegexp{regexp.MustCompile("^# Filesort: (?P<filesort>[[:alpha:]]+)  Filesort_on_disk: (?P<filesort_on_disk>[[:alpha:]]+)  Merge_passes: (?P<merge_passes>[0-9]+).*$")}
	reInnodbUsage1     = htutil.ExtRegexp{regexp.MustCompile("^# +InnoDB_IO_r_ops: (?P<io_r_ops>[0-9]+)  InnoDB_IO_r_bytes: (?P<io_r_bytes>[0-9]+)  InnoDB_IO_r_wait: (?P<io_r_wait>[0-9.]+).*$")}
	reInnodbUsage2     = htutil.ExtRegexp{regexp.MustCompile("^# +InnoDB_rec_lock_wait: (?P<rec_lock_wait>[0-9.]+)  InnoDB_queue_wait: (?P<queue_wait>[0-9.]+).*$")}
	reInnodbUsage3     = htutil.ExtRegexp{regexp.MustCompile("^# +InnoDB_pages_distinct: (?P<pages_distinct>[0-9]+).*")}
	reSetTime          = htutil.ExtRegexp{regexp.MustCompile("^SET timestamp=(?P<unixTime>[0-9]+);$")}
	reQuery            = htutil.ExtRegexp{regexp.MustCompile("^(?P<query>[^#]*).*$")}
	reUse              = htutil.ExtRegexp{regexp.MustCompile("^(?i)use ")}

	// if 'flush logs' is run at the mysql prompt (which rds commonly does, apparently) the following shows up in slow query log:
	//   /usr/local/Cellar/mysql/5.7.12/bin/mysqld, Version: 5.7.12 (Homebrew). started with:
	//   Tcp port: 3306  Unix socket: /tmp/mysql.sock
	//   Time                 Id Command    Argument
	reMySQLVersion       = htutil.ExtRegexp{regexp.MustCompile("/.*, Version: .* .*MySQL Community Server.*")}
	reMySQLPortSock      = htutil.ExtRegexp{regexp.MustCompile("Tcp port:.* Unix socket:.*")}
	reMySQLColumnHeaders = htutil.ExtRegexp{regexp.MustCompile("Time.*Id.*Command.*Argument.*")}
)

const timeFormat = "2006-01-02T15:04:05.000000"

type Config struct {
	Host          string
	User          string
	Pass          string
	QueryInterval uint
}

func Configure(v *sx.Value) htparser.SetupFunc {
	v.Map(func(m sx.Map) {})  // We don't have any configuration fields.
	return Setup
}

func Setup() (htparser.StartFunc, error) {
	// This parser doesn't have anything to set up.
	return htparser_structured.NewStartFunc(Build), nil
}

func Build(channelSize int) htparser_structured.Components {
	lineGroupChannel := make(chan []string, channelSize)
	closeChannel := func() {
		close(lineGroupChannel)
	}

	preParser := func(lineChannel <-chan string, sampler htparser.Sampler) {
		grouper(lineChannel, lineGroupChannel, sampler)
	}

	parser := func(sendEvent htparser.SendEvent) {
		threadLocalNormalizer := &normalizer.Parser{}
		for lineGroup := range lineGroupChannel {
			data, timestamp := handleEvent(threadLocalNormalizer, lineGroup)

			if data == nil || len(data) == 0 {
				continue
			}
			if q, ok := data["query"]; !ok || q == "" {
				// skip events with no query field
				continue
			}

			sendEvent(timestamp, data)
		}
	}

	return htparser_structured.Components{closeChannel, preParser, parser}
}

// A single log event can be multiple lines.  Read from 'lineChannel', find the group
// of lines that comprise an event, and write that group to 'lineGroupChannel'.
func grouper(lineChannel <-chan string, lineGroupChannel chan<- []string, sampler htparser.Sampler) {
	// flag to indicate when we've got a complete event to send
	var foundStatement bool
	groupedLines := make([]string, 0, 5)
	for line := range lineChannel {
		lineIsComment := strings.HasPrefix(line, "# ")
		if !lineIsComment && !isMySQLHeaderLine(line) {
			// we've finished the comments before the statement and now should slurp
			// lines until the next comment
			foundStatement = true
		} else {
			if foundStatement {
				// we've started a new event. Send the previous one.
				foundStatement = false
				if sampler.ShouldKeep() {
					lineGroupChannel <- groupedLines
				}
				groupedLines = make([]string, 0, 5)
			}
		}
		groupedLines = append(groupedLines, line)
	}
	// send the last event, if there was one collected
	if foundStatement {
		// if sampling is disabled or sampler says keep, pass along this group.
		if sampler.ShouldKeep() {
			lineGroupChannel <- groupedLines
		}
	}
	logrus.Debug("lines channel is closed, ending mysql processor")
}

func isMySQLHeaderLine(line string) bool {
	first := line[0]
	return (first == '/' && reMySQLVersion.MatchString(line)) ||
		(first == 'T' && reMySQLPortSock.MatchString(line)) ||
		(first == 'T' && reMySQLColumnHeaders.MatchString(line))
}

// Parse a set of MySQL log lines that seem to represent a single event and
// return a struct of extracted data as well as the highest-resolution timestamp
// available.
func handleEvent(normalizer *normalizer.Parser, rawE []string) (
	map[string]interface{}, time.Time) {
	sq := map[string]interface{}{}
	var timeFromComment time.Time
	var timeFromSet int64
	query := ""
	for _, line := range rawE {
		// parse each line and populate the map of attributes
		if _, mg := reTime.FindStringSubmatchMap(line); mg != nil {
			timeFromComment, _ = time.Parse(timeFormat, mg["time"])
		} else if reAdminPing.MatchString(line) {
			// this event is an administrative ping and we should
			// ignore the entire event
			logrus.WithFields(logrus.Fields{
				"line":  line,
				"event": rawE,
			}).Debug("readmin ping detected; skipping this event")
			return nil, time.Time{}
		} else if _, mg := reUser.FindStringSubmatchMap(line); mg != nil {
			query = ""
			sq[userKey] = strings.Split(mg["user"], "[")[0]
			sq[clientKey] = strings.TrimSpace(mg["host"])
		} else if _, mg := reQueryStats.FindStringSubmatchMap(line); mg != nil {
			query = ""
			if queryTime, err := strconv.ParseFloat(mg["queryTime"], 64); err == nil {
				sq[queryTimeKey] = queryTime
			}
			if lockTime, err := strconv.ParseFloat(mg["lockTime"], 64); err == nil {
				sq[lockTimeKey] = lockTime
			}
			if rowsSent, err := strconv.Atoi(mg["rowsSent"]); err == nil {
				sq[rowsSentKey] = rowsSent
			}
			if rowsExamined, err := strconv.Atoi(mg["rowsExamined"]); err == nil {
				sq[rowsExaminedKey] = rowsExamined
			}
			if rowsAffected, err := strconv.Atoi(mg["rowsAffected"]); err == nil {
				sq[rowsAffectedKey] = rowsAffected
			}
		} else if _, mg := reServStats.FindStringSubmatchMap(line); mg != nil {
			query = ""
			if bytesSent, err := strconv.Atoi(mg["bytesSent"]); err == nil {
				sq[bytesSentKey] = bytesSent
			}
			if tmpTables, err := strconv.Atoi(mg["tmpTables"]); err == nil {
				sq[tmpTablesKey] = tmpTables
			}
			if tmpDiskTables, err := strconv.Atoi(mg["tmpDiskTables"]); err == nil {
				sq[tmpDiskTablesKey] = tmpDiskTables
			}
			if tmpTableSizes, err := strconv.Atoi(mg["tmpTableSizes"]); err == nil {
				sq[tmpTableSizesKey] = tmpTableSizes
			}
		} else if _, mg := reInnodbQueryPlan1.FindStringSubmatchMap(line); mg != nil {
			sq[queryCacheHitKey] = mg["query_cache_hit"] == "Yes"
			sq[fullScanKey] = mg["full_scan"] == "Yes"
			sq[fullJoinKey] = mg["full_join"] == "Yes"
			sq[tmpTableKey] = mg["tmp_table"] == "Yes"
			sq[tmpTableOnDiskKey] = mg["tmp_table_on_disk"] == "Yes"
		} else if _, mg := reInnodbQueryPlan2.FindStringSubmatchMap(line); mg != nil {
			sq[fileSortKey] = mg["filesort"] == "Yes"
			sq[fileSortOnDiskKey] = mg["filesort_on_disk"] == "Yes"
			if mergePasses, err := strconv.Atoi(mg["merge_passes"]); err == nil {
				sq[mergePassesKey] = mergePasses
			}
		} else if _, mg := reInnodbUsage1.FindStringSubmatchMap(line); mg != nil {
			if ioROps, err := strconv.Atoi(mg["io_r_ops"]); err == nil {
				sq[ioROpsKey] = ioROps
			}
			if ioRBytes, err := strconv.Atoi(mg["io_r_bytes"]); err == nil {
				sq[ioRBytesKey] = ioRBytes
			}
			if ioRWait, err := strconv.ParseFloat(mg["io_r_wait"], 64); err == nil {
				sq[ioRWaitKey] = ioRWait
			}
		} else if _, mg := reInnodbUsage2.FindStringSubmatchMap(line); mg != nil {
			if recLockWait, err := strconv.ParseFloat(mg["rec_lock_wait"], 64); err == nil {
				sq[recLockWaitKey] = recLockWait
			}
			if queueWait, err := strconv.ParseFloat(mg["queue_wait"], 64); err == nil {
				sq[queueWaitKey] = queueWait
			}
		} else if _, mg := reInnodbUsage3.FindStringSubmatchMap(line); mg != nil {
			if pagesDistinct, err := strconv.Atoi(mg["pages_distinct"]); err == nil {
				sq[pagesDistinctKey] = pagesDistinct
			}
		} else if _, mg := reInnodbTrx.FindStringSubmatchMap(line); mg != nil {
			sq[transactionIDKey] = mg["trxId"]
		} else if match := reUse.FindString(line); match != "" {
			query = ""
			db := strings.TrimPrefix(line, match)
			db = strings.TrimRight(db, ";")
			db = strings.Trim(db, "`")
			sq[databaseKey] = db
			// Use this line as the query/normalized_query unless, if a real query follows it will be replaced.
			sq[queryKey] = strings.TrimRight(line, ";")
			sq[normalizedQueryKey] = sq[queryKey]
		} else if _, mg := reSetTime.FindStringSubmatchMap(line); mg != nil {
			query = ""
			timeFromSet, _ = strconv.ParseInt(mg["unixTime"], 10, 64)
		} else if isMySQLHeaderLine(line) {
			// ignore and skip the header lines
		} else if _, mg := reQuery.FindStringSubmatchMap(line); mg != nil {
			query = query + " " + mg["query"]
			if strings.HasSuffix(query, ";") {
				q := strings.TrimSpace(strings.TrimSuffix(query, ";"))
				sq[queryKey] = q
				sq[normalizedQueryKey] = normalizer.NormalizeQuery(q)
				if len(normalizer.LastTables) > 0 {
					sq[tablesKey] = strings.Join(normalizer.LastTables, " ")
				}
				if len(normalizer.LastComments) > 0 {
					sq[commentsKey] = "/* " + strings.Join(normalizer.LastComments, " */ /* ") + " */"
				}
				sq[statementKey] = normalizer.LastStatement
				query = ""
			}
		} else {
			// unknown row; log and skip
			logrus.WithFields(logrus.Fields{
				"line": line,
			}).Debug("No regex match for line in the middle of a query. skipping")
		}
	}

	// We always need a timestamp.
	//
	// timeFromComment may include millisecond resolution but doesn't include
	//   time zone.
	// timeFromSet is a UNIX timestamp and thus more reliable, but also (thus)
	//   doesn't contain millisecond resolution.
	//
	// In the best case (we have both), we combine the two; in the worst case (we
	//   have neither) we fall back to the zero value.
	combinedTime := time.Time{}
	if !timeFromComment.IsZero() && timeFromSet > 0 {
		nanos := time.Duration(timeFromComment.Nanosecond())
		combinedTime = time.Unix(timeFromSet, 0).Add(nanos)
	} else if !timeFromComment.IsZero() {
		combinedTime = timeFromComment // cross our fingers that UTC is ok
	} else if timeFromSet > 0 {
		combinedTime = time.Unix(timeFromSet, 0)
	}

	return sq, combinedTime
}
