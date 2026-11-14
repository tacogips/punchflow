package webhook

import "time"

// Backoff returns exponential retry delay, capped at one hour.
func Backoff(base time.Duration, retry int) time.Duration {
	if retry < 1 || base <= 0 {
		return 0
	}
	delay := base
	for n := 1; n < retry && delay < time.Hour; n++ {
		if delay > time.Hour/2 {
			return time.Hour
		}
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
