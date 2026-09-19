package typesafe

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

const DefaultTimeout = 10 * time.Second

type RetryPolicy struct {
	MaxRetries         int
	BackoffInitial     time.Duration
	BackoffMax         time.Duration
	BackoffJitter      float64
	HTTPStatuses       map[int]bool
	RespectRetryAfter  bool
	MaxRetryAfter      time.Duration
	APIConnectionError bool
	APITimeoutError    bool
}

func DefaultRetryPolicy() RetryPolicy {
	statuses := map[int]bool{408: true, 429: true}
	for status := 500; status < 600; status++ {
		statuses[status] = true
	}
	return RetryPolicy{
		MaxRetries: 2, BackoffInitial: 500 * time.Millisecond, BackoffMax: 5 * time.Second,
		BackoffJitter: .25, HTTPStatuses: statuses, RespectRetryAfter: true,
		MaxRetryAfter: time.Minute, APIConnectionError: true, APITimeoutError: true,
	}
}

func (p RetryPolicy) clone() RetryPolicy {
	copy := p
	copy.HTTPStatuses = make(map[int]bool, len(p.HTTPStatuses))
	for status, enabled := range p.HTTPStatuses {
		copy.HTTPStatuses[status] = enabled
	}
	return copy
}

// ParseRetryAfter parses retry-after-ms or Retry-After, preferring milliseconds.
func ParseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	if raw, exists := header[http.CanonicalHeaderKey("retry-after-ms")]; exists && len(raw) > 0 {
		if ms, err := strconv.ParseFloat(raw[0], 64); err == nil && ms >= 0 {
			return time.Duration(ms * float64(time.Millisecond)), true
		}
	}
	raw := header.Get("Retry-After")
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds * float64(time.Second)), true
	}
	date, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	delay := date.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

// RetryDelay calculates capped exponential backoff for a zero-based attempt.
func RetryDelay(attempt int, header http.Header, policy RetryPolicy, random func() float64) time.Duration {
	if policy.RespectRetryAfter && header != nil {
		if delay, ok := ParseRetryAfter(header, time.Now()); ok && delay <= policy.MaxRetryAfter {
			return delay
		}
	}
	exponential := float64(policy.BackoffInitial) * math.Pow(2, float64(attempt))
	if exponential > float64(policy.BackoffMax) {
		exponential = float64(policy.BackoffMax)
	}
	return time.Duration(math.Round(exponential * (1 - random()*policy.BackoffJitter)))
}
