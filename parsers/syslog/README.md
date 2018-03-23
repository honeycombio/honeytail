# syslog parser

Example CLI usage (from honeytail root)
```
honeytail -p syslog -k $HONEYTAIL_WRITEKEY \
  -f /var/log/auth.log \
  --dataset 'MY_TEST_DATASET' \
  --syslog.mode 'rfc5424'
```

## Log Formatting

You will need to configure your syslog daemon to use the right format. For example, to use RFC5424 with rsyslog, set the following in your /etc/rsyslog.conf.

```
$ActionFileDefaultTemplate RSYSLOG_SyslogProtocol23Format
```

__RFC5424__

[RFC Text](https://www.ietf.org/rfc/rfc5424.txt)

Example line

```
<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry...
```

__RFC3164__

[RFC Text](https://www.ietf.org/rfc/rfc3164.txt)

Example line

```
<34>Oct 11 22:14:15 mymachine su: 'su root' failed for user on /dev/pts/8
```
