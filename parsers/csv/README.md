Example CLI usage (from honeytail root)
```
honeytail -p csv -k $HONEYTAIL_WRITEKEY \
  -f some/path/system.log \
  --dataset 'MY_TEST_DATASET' \
  --backfill \
  --csv.fields="time,field_1,field_2,field_3"
  --csv.timefield="time" \
  --csv.time_format="%H:%M:%S"
```
