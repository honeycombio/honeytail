# syslog parser

Example CLI usage (from honeytail root)
```
honeytail -p syslog -k $HONEYTAIL_WRITEKEY \
  -f /var/log/auth.log \
  --dataset 'MY_TEST_DATASET' \
  --syslog.mode 'rfc5424'
```

## other notes

You will need to configure your syslog daemon to use the right format. For example, to use RFC5424 with rsyslog, set the following in your /etc/rsyslog.conf.

```
$ActionFileDefaultTemplate RSYSLOG_SyslogProtocol23Format
```