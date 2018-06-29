package logparser_test

import (
	"encoding/json"
	"testing"

	"github.com/honeycombio/mongodbtools/logparser"
	"github.com/stretchr/testify/assert"
)

func TestParseLogLine(t *testing.T) {
	testCases := []struct {
		line   string
		output string
	}{
		{
			line:   "Mon Feb 23 03:20:19.670 [TTLMonitor] query local.system.indexes query: { expireAfterSeconds: { $exists: true } } ntoreturn:0 ntoskip:0 nscanned:0 keyUpdates:0 locks(micros) r:86 nreturned:0 reslen:20 0ms",
			output: `{"context":"TTLMonitor","duration_ms":0,"keyUpdates":0,"locks(micros)":{"r":86},"namespace":"local.system.indexes","nreturned":0,"nscanned":0,"ntoreturn":0,"ntoskip":0,"operation":"query","query":{"expireAfterSeconds":{"$exists":true}},"reslen":20,"timestamp":"Mon Feb 23 03:20:19.670"}`,
		},
		{
			line:   "2017-08-14T00:09:17.028-0400 I COMMAND  [conn555555] query foo.bar query: { $query: { fieldA: /^123456789.?\\/test.?$/ } } planSummary: IXSCAN { fieldA: 1, fieldB: 1 } ntoreturn:0 ntoskip:0 nscanned:2 nscannedObjects:1 keyUpdates:0 writeConflicts:0 numYields:1 nreturned:1 reslen:1337 locks:{ Global: { acquireCount: { r: 4 } }, Database: { acquireCount: { r: 2 } }, Collection: { acquireCount: { r: 2 } } } 134ms",
			output: `{"component":"COMMAND","context":"conn555555","duration_ms":134,"keyUpdates":0,"locks":{"Collection":{"acquireCount":{"r":2}},"Database":{"acquireCount":{"r":2}},"Global":{"acquireCount":{"r":4}}},"namespace":"foo.bar","nreturned":1,"nscanned":2,"nscannedObjects":1,"ntoreturn":0,"ntoskip":0,"numYields":1,"operation":"query","planSummary":[{"IXSCAN":{"fieldA":1,"fieldB":1}}],"query":{"$query":{"fieldA":"/^123456789.?\\/test.?$/"}},"reslen":1337,"severity":"informational","timestamp":"2017-08-14T00:09:17.028-0400","writeConflicts":0}`,
		},
	}
	for _, tc := range testCases {
		doc, err := logparser.ParseLogLine(tc.line)
		assert.NoError(t, err)
		buf, _ := json.Marshal(doc)
		assert.Equal(t, string(buf), tc.output)
	}
}
