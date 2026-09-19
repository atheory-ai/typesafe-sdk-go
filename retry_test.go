package typesafe

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 2 || p.BackoffInitial != 500*time.Millisecond ||
		p.BackoffMax != 5*time.Second || p.BackoffJitter != .25 ||
		!p.HTTPStatuses[408] || !p.HTTPStatuses[429] || !p.HTTPStatuses[599] ||
		p.MaxRetryAfter != time.Minute || !p.RespectRetryAfter ||
		!p.APIConnectionError || !p.APITimeoutError {
		t.Fatalf("unexpected defaults: %#v", p)
	}
}

func TestSleepContext(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestParseRetryAfterEdgeCases(t *testing.T) {
	now := time.Now()
	if _, ok := ParseRetryAfter(http.Header{"Retry-After": {"-1"}}, now); ok {
		t.Fatal("accepted negative seconds")
	}
	if got, ok := ParseRetryAfter(http.Header{"Retry-After": {now.Add(-time.Hour).Format(http.TimeFormat)}}, now); !ok || got != 0 {
		t.Fatalf("past date: %v %v", got, ok)
	}
	if _, ok := ParseRetryAfter(http.Header{}, now); ok {
		t.Fatal("accepted missing header")
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		headers http.Header
		want    time.Duration
		ok      bool
	}{
		{http.Header{"Retry-After-Ms": {"125"}, "Retry-After": {"9"}}, 125 * time.Millisecond, true},
		{http.Header{"Retry-After": {"1.5"}}, 1500 * time.Millisecond, true},
		{http.Header{"Retry-After": {now.Add(2 * time.Second).Format(http.TimeFormat)}}, 2 * time.Second, true},
		{http.Header{"Retry-After": {"garbage"}}, 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseRetryAfter(tt.headers, now)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseRetryAfter(%v) = %v, %v; want %v, %v", tt.headers, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	p := DefaultRetryPolicy()
	if got := RetryDelay(0, nil, p, func() float64 { return 0 }); got != 500*time.Millisecond {
		t.Fatalf("attempt zero: %v", got)
	}
	if got := RetryDelay(8, nil, p, func() float64 { return 0 }); got != 5*time.Second {
		t.Fatalf("capped delay: %v", got)
	}
	if got := RetryDelay(0, nil, p, func() float64 { return 1 }); got != 375*time.Millisecond {
		t.Fatalf("jittered delay: %v", got)
	}
}
