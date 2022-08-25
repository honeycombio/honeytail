package logparser_test

import (
	"encoding/json"
	"testing"

	"github.com/honeycombio/honeytail/parsers/mongodb/logparser"
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
		{
			line:   "2020-10-22T01:55:27.585+0000 I COMMAND [conn111918] command cm.auditLog command: find { find: \"auditLog\", filter: { resourceLevelInfos.resourceId: { $in: [ \"5f90e685c8352e00014abb3f\" ] } }, projection: {}, $db: \"crisismanagement\", $clusterTime: { clusterTime: Timestamp(1603331719, 18), signature: { hash: BinData(0, 164ACBB1F99D1E544AAC2497483A20E6FEAF126A), keyId: 6860053861185355777 } }, lsid: { id: UUID(\"bac26ad1-3d76-4da1-94cc-d541942f6889\") } } planSummary: COLLSCAN keysExamined:0 docsExamined:4765871 cursorExhausted:1 numYields:37237 nreturned:2 reslen:3038 locks:{ Global: { acquireCount: { r: 37238 } }, Database: { acquireCount: { r: 37238 } }, Collection: { acquireCount: { r: 37238 } } } storage:{ data: { bytesRead: 485564359, timeReadingMicros: 366237 } } protocol:op_msg 7616ms",
			output: `{"command":{"$clusterTime":{"clusterTime":"Timestamp(1603331719, 18)","signature":{"hash":"BinData(0, 164ACBB1F99D1E544AAC2497483A20E6FEAF126A)","keyId":6860053861185356000}},"$db":"crisismanagement","filter":{"resourceLevelInfos.resourceId":{"$in":["5f90e685c8352e00014abb3f"]}},"find":"auditLog","lsid":{"id":"UUID(\"bac26ad1-3d76-4da1-94cc-d541942f6889\")"},"projection":{}},"command_type":"find","component":"COMMAND","context":"conn111918","cursorExhausted":1,"docsExamined":4765871,"duration_ms":7616,"keysExamined":0,"locks":{"Collection":{"acquireCount":{"r":37238}},"Database":{"acquireCount":{"r":37238}},"Global":{"acquireCount":{"r":37238}}},"namespace":"cm.auditLog","nreturned":2,"numYields":37237,"operation":"command","planSummary":[{"COLLSCAN":true}],"protocol":"op_msg","reslen":3038,"severity":"informational","storage":{"data":{"bytesRead":485564359,"timeReadingMicros":366237}},"timestamp":"2020-10-22T01:55:27.585+0000"}`,
		},
	}
	for _, tc := range testCases {
		doc, err := logparser.ParseLogLine(tc.line)
		assert.NoError(t, err)
		buf, _ := json.Marshal(doc)
		assert.Equal(t, string(buf), tc.output)
	}
}
