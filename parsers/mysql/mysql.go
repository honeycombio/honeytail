// Package mysql parses the mysql slow query log
package mysql

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	_ "github.com/go-sql-driver/mysql"
	"github.com/honeycombio/mysqltools/query/normalizer"

	"github.com/honeycombio/honeytail/event"
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
	rdsStr  = "rds"
	ec2Str  = "ec2"
	selfStr = "self"

	// Event attributes
	userKey            = "user"
	clientKey          = "client"
	clientIPKey        = "client_ip"
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
	// Event attributes that apply to the host as a whole
	hostedOnKey   = "hosted_on"
	readOnlyKey   = "read_only"
	replicaLagKey = "replica_lag"
	roleKey       = "role"
)

var (
	reTime             = myRegexp{regexp.MustCompile("^# Time: (?P<time>[^ ]+)Z *$")}
	reAdminPing        = myRegexp{regexp.MustCompile("^# administrator command: Ping; *$")}
	reUser             = myRegexp{regexp.MustCompile("^# User@Host: (?P<user>[^#]+) @ (?P<host>[^#]+).*$")}
	reQueryStats       = myRegexp{regexp.MustCompile("^# Query_time: (?P<queryTime>[0-9.]+) *Lock_time: (?P<lockTime>[0-9.]+) *Rows_sent: (?P<rowsSent>[0-9]+) *Rows_examined: (?P<rowsExamined>[0-9]+)( *Rows_affected: (?P<rowsAffected>[0-9]+))?.*$")}
	reServStats        = myRegexp{regexp.MustCompile("^# Bytes_sent: (?P<bytesSent>[0-9.]+) *Tmp_tables: (?P<tmpTables>[0-9.]+) *Tmp_disk_tables: (?P<tmpDiskTables>[0-9]+) *Tmp_table_sizes: (?P<tmpTableSizes>[0-9]+).*$")}
	reInnodbTrx        = myRegexp{regexp.MustCompile("^# InnoDB_trx_id: (?P<trxId>[A-F0-9]+) *$")}
	reInnodbQueryPlan1 = myRegexp{regexp.MustCompile("^# QC_Hit: (?P<query_cache_hit>[[:alpha:]]+)  Full_scan: (?P<full_scan>[[:alpha:]]+)  Full_join: (?P<full_join>[[:alpha:]]+)  Tmp_table: (?P<tmp_table>[[:alpha:]]+)  Tmp_table_on_disk: (?P<tmp_table_on_disk>[[:alpha:]]+).*$")}
	reInnodbQueryPlan2 = myRegexp{regexp.MustCompile("^# Filesort: (?P<filesort>[[:alpha:]]+)  Filesort_on_disk: (?P<filesort_on_disk>[[:alpha:]]+)  Merge_passes: (?P<merge_passes>[0-9]+).*$")}
	reInnodbUsage1     = myRegexp{regexp.MustCompile("^# +InnoDB_IO_r_ops: (?P<io_r_ops>[0-9]+)  InnoDB_IO_r_bytes: (?P<io_r_bytes>[0-9]+)  InnoDB_IO_r_wait: (?P<io_r_wait>[0-9.]+).*$")}
	reInnodbUsage2     = myRegexp{regexp.MustCompile("^# +InnoDB_rec_lock_wait: (?P<rec_lock_wait>[0-9.]+)  InnoDB_queue_wait: (?P<queue_wait>[0-9.]+).*$")}
	reInnodbUsage3     = myRegexp{regexp.MustCompile("^# +InnoDB_pages_distinct: (?P<pages_distinct>[0-9]+).*")}
	reSetTime          = myRegexp{regexp.MustCompile("^SET timestamp=(?P<unixTime>[0-9]+);$")}
	reQuery            = myRegexp{regexp.MustCompile("^(?P<query>[^#]*).*$")}
	reUse              = myRegexp{regexp.MustCompile("^(?i)use ")}

	// if 'flush logs' is run at the mysql prompt (which rds commonly does, apparently) the following shows up in slow query log:
	//   /usr/local/Cellar/mysql/5.7.12/bin/mysqld, Version: 5.7.12 (Homebrew). started with:
	//   Tcp port: 3306  Unix socket: /tmp/mysql.sock
	//   Time                 Id Command    Argument
	reMySQLVersion       = myRegexp{regexp.MustCompile("/.*, Version: .* .*MySQL Community Server.*")}
	reMySQLPortSock      = myRegexp{regexp.MustCompile("Tcp port:.* Unix socket:.*")}
	reMySQLColumnHeaders = myRegexp{regexp.MustCompile("Time.*Id.*Command.*Argument.*")}
)

const timeFormat = "2006-01-02T15:04:05.000000"

type Options struct {
	Host          string `long:"host" description:"MySQL host in the format (address:port)"`
	User          string `long:"user" description:"MySQL username"`
	Pass          string `long:"pass" description:"MySQL password"`
	QueryInterval uint   `long:"interval" description:"interval for querying the MySQL DB in seconds" default:"30"`
}

type parserStats struct {
	LinesEncountered  int
	LinesErrored      int
	EventsParsed      int
	EventsSent        int
	EventMissingQuery int
	EventSkipped      int

	lock sync.Mutex
}

type Parser struct {
	conf       Options
	wg         sync.WaitGroup
	nower      Nower
	hostedOn   string
	readOnly   *bool
	replicaLag *int64
	role       *string
	normalizer *normalizer.Parser
	stats      parserStats
}

type Nower interface {
	Now() time.Time
}

type RealNower struct{}

func (n *RealNower) Now() time.Time {
	return time.Now().UTC()
}

func (p *Parser) Init(options interface{}) error {
	p.conf = *options.(*Options)
	p.nower = &RealNower{}
	p.normalizer = &normalizer.Parser{}
	if p.conf.Host != "" {
		url := fmt.Sprintf("%s:%s@tcp(%s)/", p.conf.User, p.conf.Pass, p.conf.Host)
		db, err := sql.Open("mysql", url)
		if err != nil {
			logrus.Fatal(err)
		}

		// run one time queries
		hostedOn, err := getHostedOn(db)
		if err != nil {
			logrus.WithError(err).Warn("failed to get host env")
		}
		p.hostedOn = hostedOn

		role, err := getRole(db)
		if err != nil {
			logrus.WithError(err).Warn("failed to get role")
		}
		p.role = role

		// update hostedOn and readOnly every <n> seconds
		go func() {
			defer db.Close()
			ticker := time.NewTicker(time.Second * time.Duration(p.conf.QueryInterval))
			for _ = range ticker.C {
				readOnly, err := getReadOnly(db)
				if err != nil {
					logrus.WithError(err).Warn("failed to get read-only state")
				}
				p.readOnly = readOnly

				replicaLag, err := getReplicaLag(db)
				if err != nil {
					logrus.WithError(err).Warn("failed to get replica lag")
				}
				p.replicaLag = replicaLag
			}
		}()
	}
	return nil
}

func (p *Parser) LogStats() {
	logrus.WithFields(logrus.Fields{
		"lines_encountered":    p.stats.LinesEncountered,
		"lines_errored":        p.stats.LinesErrored,
		"events_parsed":        p.stats.EventsParsed,
		"events_sent":          p.stats.EventsSent,
		"events_missing_query": p.stats.EventMissingQuery,
		"events_skipped":       p.stats.EventSkipped,
	}).Info("mysql parser stats")
	p.resetStats()
}

func (p *Parser) resetStats() {
	p.stats.lock.Lock()
	defer p.stats.lock.Unlock()
	p.stats.LinesEncountered = 0
	p.stats.LinesErrored = 0
	p.stats.EventsParsed = 0
	p.stats.EventsSent = 0
	p.stats.EventMissingQuery = 0
	p.stats.EventSkipped = 0
}

func getReadOnly(db *sql.DB) (*bool, error) {
	rows, err := db.Query("SELECT @@global.read_only;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var value bool

	rows.Next()
	if err := rows.Scan(&value); err != nil {
		logrus.WithError(err).Warn("failed to get read-only state")
		return nil, err
	}
	if err := rows.Err(); err != nil {
		logrus.WithError(err).Warn("failed to get read-only state")
		return nil, err
	}
	return &value, nil
}

func getHostedOn(db *sql.DB) (string, error) {
	rows, err := db.Query("SHOW GLOBAL VARIABLES WHERE Variable_name = 'basedir';")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var varName, value string

	for rows.Next() {
		if err := rows.Scan(&varName, &value); err != nil {
			return "", err
		}
		if strings.HasPrefix(value, "/rdsdbbin/") {
			return rdsStr, nil
		}
	}

	// TODO: implement ec2 detection

	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func getReplicaLag(db *sql.DB) (*int64, error) {
	rows, err := db.Query("SHOW SLAVE STATUS")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, err
	}

	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))
	for i := range values {
		var v sql.RawBytes
		values[i] = &v
	}

	err = rows.Scan(values...)
	if err != nil {
		return nil, err
	}

	for i, name := range columns {
		bp := values[i].(*sql.RawBytes)
		vs := string(*bp)
		if name == "Seconds_Behind_Master" {
			vi, err := strconv.ParseInt(vs, 10, 64)
			if err != nil {
				return nil, err
			}
			return &vi, nil
		}
	}
	return nil, nil
}

func getRole(db *sql.DB) (*string, error) {
	var res string
	rows, err := db.Query("SHOW MASTER STATUS")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		res = "secondary"
		return nil, err
	}
	res = "primary"
	return &res, nil
}

func isMySQLHeaderLine(line string) bool {
	first := line[0]
	return (first == '/' && reMySQLVersion.MatchString(line)) ||
		(first == 'T' && reMySQLPortSock.MatchString(line)) ||
		(first == 'T' && reMySQLColumnHeaders.MatchString(line))
}

func (p *Parser) ProcessLines(lines <-chan string, send chan<- event.Event) {
	// start up a goroutine to handle grouped sets of lines
	rawEvents := make(chan []string)
	var wg sync.WaitGroup
	p.wg = wg
	defer p.wg.Wait()
	p.wg.Add(1)
	go p.handleEvents(rawEvents, send)

	// flag to indicate when we've got a complete event to send
	var foundStatement bool
	groupedLines := make([]string, 0, 5)
	for line := range lines {
		p.stats.LinesEncountered++
		lineIsComment := strings.HasPrefix(line, "# ")
		if !lineIsComment && !isMySQLHeaderLine(line) {
			// we've finished the comments before the statement and now should slurp
			// lines until the next comment
			foundStatement = true
		} else {
			if foundStatement {
				// we've started a new event. Send the previous one.
				foundStatement = false
				rawEvents <- groupedLines
				groupedLines = make([]string, 0, 5)
			}
		}
		groupedLines = append(groupedLines, line)
	}
	// send the last event, if there was one collected
	if foundStatement {
		p.stats.EventsParsed++
		rawEvents <- groupedLines
	}
	logrus.Debug("lines channel is closed, ending mysql processor")
	close(rawEvents)
}

func (p *Parser) handleEvents(rawEvents <-chan []string, send chan<- event.Event) {
	defer p.wg.Done()
	for rawE := range rawEvents {
		sq, timestamp := p.handleEvent(rawE)
		if len(sq) == 0 {
			p.stats.EventSkipped++
			continue
		}
		if q, ok := sq["query"]; !ok || q == "" {
			// count and skip events with no query field
			p.stats.EventMissingQuery++
			continue
		}
		if p.hostedOn != "" {
			sq[hostedOnKey] = p.hostedOn
		}
		if p.readOnly != nil {
			sq[readOnlyKey] = *p.readOnly
		}
		if p.replicaLag != nil {
			sq[replicaLagKey] = *p.replicaLag
		}
		if p.role != nil {
			sq[roleKey] = *p.role
		}
		p.stats.EventsSent++
		send <- event.Event{
			Timestamp: timestamp,
			Data:      sq,
		}
	}
	logrus.Debug("done with mysql handleEvents")
}

// Parse a set of MySQL log lines that seem to represent a single event and
// return a struct of extracted data as well as the highest-resolution timestamp
// available.
func (p *Parser) handleEvent(rawE []string) (map[string]interface{}, time.Time) {
	sq := map[string]interface{}{}
	var timeFromComment time.Time
	var timeFromSet int64
	query := ""
	for _, line := range rawE {
		// parse each line and populate the map of attributes
		if mg := reTime.FindStringSubmatchMap(line); mg != nil {
			timeFromComment, _ = time.Parse(timeFormat, mg["time"])
		} else if reAdminPing.MatchString(line) {
			// this event is an administrative ping and we should
			// ignore the entire event
			logrus.WithFields(logrus.Fields{
				"line":  line,
				"event": rawE,
			}).Debug("readmin ping detected; skipping this event")
			return nil, time.Time{}
		} else if mg := reUser.FindStringSubmatchMap(line); mg != nil {
			query = ""
			sq[userKey] = strings.Split(mg["user"], "[")[0]
			hostAndIP := strings.Split(mg["host"], " ")
			switch len(hostAndIP) {
			case 0: // do nothing
			case 1:
				sq[clientIPKey] = hostAndIP[0][1 : len(hostAndIP[0])-1]
			default:
				sq[clientKey] = hostAndIP[0]
				sq[clientIPKey] = hostAndIP[1][1 : len(hostAndIP[1])-1]
			}
		} else if mg := reQueryStats.FindStringSubmatchMap(line); mg != nil {
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
		} else if mg := reServStats.FindStringSubmatchMap(line); mg != nil {
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
		} else if mg := reInnodbQueryPlan1.FindStringSubmatchMap(line); mg != nil {
			sq[queryCacheHitKey] = mg["query_cache_hit"] == "Yes"
			sq[fullScanKey] = mg["full_scan"] == "Yes"
			sq[fullJoinKey] = mg["full_join"] == "Yes"
			sq[tmpTableKey] = mg["tmp_table"] == "Yes"
			sq[tmpTableOnDiskKey] = mg["tmp_table_on_disk"] == "Yes"
		} else if mg := reInnodbQueryPlan2.FindStringSubmatchMap(line); mg != nil {
			sq[fileSortKey] = mg["filesort"] == "Yes"
			sq[fileSortOnDiskKey] = mg["filesort_on_disk"] == "Yes"
			if mergePasses, err := strconv.Atoi(mg["merge_passes"]); err == nil {
				sq[mergePassesKey] = mergePasses
			}
		} else if mg := reInnodbUsage1.FindStringSubmatchMap(line); mg != nil {
			if ioROps, err := strconv.Atoi(mg["io_r_ops"]); err == nil {
				sq[ioROpsKey] = ioROps
			}
			if ioRBytes, err := strconv.Atoi(mg["io_r_bytes"]); err == nil {
				sq[ioRBytesKey] = ioRBytes
			}
			if ioRWait, err := strconv.ParseFloat(mg["io_r_wait"], 64); err == nil {
				sq[ioRWaitKey] = ioRWait
			}
		} else if mg := reInnodbUsage2.FindStringSubmatchMap(line); mg != nil {
			if recLockWait, err := strconv.ParseFloat(mg["rec_lock_wait"], 64); err == nil {
				sq[recLockWaitKey] = recLockWait
			}
			if queueWait, err := strconv.ParseFloat(mg["queue_wait"], 64); err == nil {
				sq[queueWaitKey] = queueWait
			}
		} else if mg := reInnodbUsage3.FindStringSubmatchMap(line); mg != nil {
			if pagesDistinct, err := strconv.Atoi(mg["pages_distinct"]); err == nil {
				sq[pagesDistinctKey] = pagesDistinct
			}
		} else if mg := reInnodbTrx.FindStringSubmatchMap(line); mg != nil {
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
		} else if mg := reSetTime.FindStringSubmatchMap(line); mg != nil {
			query = ""
			timeFromSet, _ = strconv.ParseInt(mg["unixTime"], 10, 64)
		} else if isMySQLHeaderLine(line) {
			// ignore and skip the header lines
		} else if mg := reQuery.FindStringSubmatchMap(line); mg != nil {
			query = query + " " + mg["query"]
			if strings.HasSuffix(query, ";") {
				q := strings.TrimSpace(strings.TrimSuffix(query, ";"))
				sq[queryKey] = q
				sq[normalizedQueryKey] = p.normalizer.NormalizeQuery(q)
				if len(p.normalizer.LastTables) > 0 {
					sq[tablesKey] = strings.Join(p.normalizer.LastTables, " ")
				}
				if len(p.normalizer.LastComments) > 0 {
					sq[commentsKey] = "/* " + strings.Join(p.normalizer.LastComments, " */ /* ") + " */"
				}
				sq[statementKey] = p.normalizer.LastStatement
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
	//   have neither) we fall back to "now."
	combinedTime := p.nower.Now()
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

// custom error to indicate empty query
type emptyQueryError struct {
	err string
}

func (e *emptyQueryError) Error() string {
	e.err = "skipped slow query"
	return e.err
}
