package mysql

import (
	"reflect"
	"testing"
	"time"

	"github.com/honeycombio/mysqltools/query/normalizer"
)

type slowQueryData struct {
	rawE rawEvent
	sq   SlowQuery
	// processSlowQuery will error
	psqWillError bool
}

var t1, _ = time.Parse("02/Jan/2006:15:04:05.000000", "01/Apr/2016:00:31:09.817887")
var t2, _ = time.Parse("02/Jan/2006:15:04:05.000000", "02/Aug/2010:13:24:56")
var sqds = []slowQueryData{
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: 2016-04-01T00:31:09.817887Z",
				"# Query_time: 0.008393  Lock_time: 0.000154 Rows_sent: 1  Rows_examined: 357",
			},
		},
		sq: SlowQuery{
			Timestamp:    t1,
			QueryTime:    0.008393,
			LockTime:     0.000154,
			RowsSent:     1,
			RowsExamined: 357,
		},
		psqWillError: false,
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-parsable-time-stampZ",
				"# User@Host: someuser @ hostfoo [192.168.2.1]  Id:   666",
			},
		},
		sq: SlowQuery{
			Timestamp: t2,
			User:      "someuser",
			Client:    "hostfoo",
			ClientIP:  "192.168.2.1",
		},
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-parsable-time-stampZ",
				"# User@Host: root @ localhost []  Id:   233",
			},
		},
		sq: SlowQuery{
			Timestamp: t2,
			User:      "root",
			Client:    "localhost",
		},
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-recognizable time stamp",
				"# administrator command: Ping;",
			},
		},
		sq: SlowQuery{
			Timestamp: t2,
			skipQuery: true,
		},
		psqWillError: true,
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-parsable-time-stampZ",
				"SET timestamp=1459470669;",
				"show status like 'Uptime';",
			},
		},
		sq: SlowQuery{
			Timestamp:       t2,
			UnixTime:        1459470669,
			Query:           "show status like 'Uptime';",
			NormalizedQuery: "show status like ?;",
		},
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-parsable-time-stampZ",
				"SET timestamp=1459470669;",
				"SELECT * FROM (SELECT  T1.orderNumber,  STATUS,  SUM(quantityOrdered * priceEach) AS  total FROM orders WHERE total > 1000 AS T1 INNER JOIN orderdetails AS T2 ON T1.orderNumber = T2.orderNumber GROUP BY  orderNumber) T WHERE total > 100;",
			},
		},
		sq: SlowQuery{
			Timestamp:       t2,
			UnixTime:        1459470669,
			Query:           "SELECT * FROM (SELECT  T1.orderNumber,  STATUS,  SUM(quantityOrdered * priceEach) AS  total FROM orders WHERE total > 1000 AS T1 INNER JOIN orderdetails AS T2 ON T1.orderNumber = T2.orderNumber GROUP BY  orderNumber) T WHERE total > 100;",
			NormalizedQuery: "select * from (select t1.ordernumber, status, sum(quantityordered * priceeach) as total from orders where total > ? as t1 inner join orderdetails as t2 on t1.ordernumber = t2.ordernumber group by ordernumber) t where total > ?;",
		},
	},
	{
		rawE: rawEvent{
			lines: []string{
				"# Time: not-a-parsable-time-stampZ",
				"SET timestamp=1459470669;",
				"use someDB;",
			},
		},
		sq: SlowQuery{
			Timestamp: t2,
			UnixTime:  1459470669,
			DB:        "someDB",
			Query:     "use someDB;",
		},
	},
	{
		rawE: rawEvent{
			lines: []string{},
		},
		sq: SlowQuery{},
	},
}

func TestHandleEvent(t *testing.T) {
	p := &Parser{
		nower:      &FakeNower{},
		normalizer: &normalizer.Scanner{},
	}
	for i, sqd := range sqds {
		res := p.handleEvent(sqd.rawE)
		if !reflect.DeepEqual(res, sqd.sq) {
			t.Errorf("case num %d: expected\n %+v, got\n %+v", i, sqd.sq, res)
		}
	}
}

func TestProcessSlowQuery(t *testing.T) {
	p := &Parser{
		nower:      &FakeNower{},
		normalizer: &normalizer.Scanner{},
	}
	for i, sqd := range sqds {
		res, err := p.processSlowQuery(sqd.sq)
		if err == nil && sqd.psqWillError {
			t.Fatalf("case num %d: expected processSlowQuery to error (%+v) but it didn't. sq: %+v, res: %+v", i, err, sqd, res)
		}
	}
}

type FakeNower struct{}

func (f *FakeNower) Now() time.Time {
	fakeTime, _ := time.Parse("02/Jan/2006:15:04:05.000000 -0700", "02/Aug/2010:13:24:56 -0000")
	return fakeTime
}
