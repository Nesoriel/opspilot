package tlsinspect

import "time"

func remainingDays(notAfter, now time.Time) int64 {
	duration := notAfter.Sub(now)
	days := duration / (24 * time.Hour)
	if duration < 0 && duration%(24*time.Hour) != 0 {
		days--
	}
	return int64(days)
}
