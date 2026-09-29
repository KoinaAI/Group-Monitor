package main

import "time"

// Message dates use Beijing time even in UTC containers without tzdata.
var messageTimeZone = time.FixedZone("UTC+08:00", 8*60*60)

func messageTimestamp(seconds int64) string {
	if seconds <= 0 {
		return "时间未知"
	}
	return time.Unix(seconds, 0).In(messageTimeZone).Format(time.RFC3339)
}
