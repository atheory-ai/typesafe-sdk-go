package typesafe

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func noWait(client *Client) *[]time.Duration {
	delays := []time.Duration{}
	client.random = func() float64 { return 0 }
	client.sleep = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	return &delays
}

func TestRetriesStatusThenSucceeds(t *testing.T) {
	var calls int
	var retryHeaders []string
	client := newTestClient(t, func(request *http.Request) (*http.Response, error) {
		calls++
		retryHeaders = append(retryHeaders, request.Header.Get("X-TypeSafe-Retry-Count"))
		if calls < 3 {
			return response(429, `{"error":"slow"}`, http.Header{"Retry-After-Ms": {"7"}}), nil
		}
		return response(200, `{"models":[]}`), nil
	}, WithMaxRetries(2))
	delays := noWait(client)
	if _, err := client.Models.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(*delays) != 2 || (*delays)[0] != 7*time.Millisecond || retryHeaders[0] != "" || retryHeaders[1] != "1" || retryHeaders[2] != "2" {
		t.Fatalf("calls=%d delays=%v retry headers=%v", calls, *delays, retryHeaders)
	}
}

func TestRetriesConnectionErrors(t *testing.T) {
	var calls int
	client := newTestClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("reset")
		}
		return response(200, `{"models":[]}`), nil
	}, WithMaxRetries(1))
	noWait(client)
	if _, err := client.Models.List(context.Background()); err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestDoesNotRetryNonRetryableStatus(t *testing.T) {
	var calls int
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return response(400, `{"error":"bad"}`), nil }, WithMaxRetries(2))
	noWait(client)
	_, err := client.Models.List(context.Background())
	var bad *BadRequestError
	if !errors.As(err, &bad) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPerCallRetryOverride(t *testing.T) {
	var calls int
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return response(503, `{"error":"bad"}`), nil }, WithMaxRetries(2))
	noWait(client)
	_, _ = client.Models.List(context.Background(), WithRequestMaxRetries(0))
	if calls != 1 || client.Retry.MaxRetries != 2 {
		t.Fatalf("calls=%d client retries=%d", calls, client.Retry.MaxRetries)
	}
}

func TestRetryPolicyCanDisableConnectionErrors(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.MaxRetries = 2
	policy.APIConnectionError = false
	var calls int
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("reset") }, WithRetryPolicy(policy))
	noWait(client)
	_, _ = client.Models.List(context.Background())
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestTimeoutAndRetry(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}, WithTimeout(5*time.Millisecond), WithMaxRetries(1))
	client.sleep = func(context.Context, time.Duration) error { return nil }
	_, err := client.Models.List(context.Background())
	var timeout *APITimeoutError
	var connection *APIConnectionError
	if !errors.As(err, &timeout) || !errors.As(err, &connection) || calls.Load() != 2 || timeout.Timeout != 5*time.Millisecond {
		t.Fatalf("calls=%d err=%T %v", calls.Load(), err, err)
	}
}

func TestCallerCancellationIsNotRetried(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	client := newTestClient(t, func(request *http.Request) (*http.Response, error) {
		calls++
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	}, WithMaxRetries(2))
	_, err := client.Models.List(ctx)
	var aborted *APIUserAbortError
	if !errors.As(err, &aborted) || calls != 1 {
		t.Fatalf("calls=%d err=%T %v", calls, err, err)
	}
}

func TestCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(503, `{"error":"bad"}`), nil }, WithMaxRetries(2))
	client.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	_, err := client.Models.List(ctx)
	var aborted *APIUserAbortError
	if !errors.As(err, &aborted) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestRateLimitErrorRetryAfter(t *testing.T) {
	client := newTestClient(t, func(*http.Request) (*http.Response, error) {
		return response(429, `{}`, http.Header{"Retry-After": {"1.5"}}), nil
	})
	_, err := client.Models.List(context.Background())
	var rate *RateLimitError
	if !errors.As(err, &rate) || !rate.HasRetryAfter || rate.RetryAfter != 1500*time.Millisecond {
		t.Fatalf("%T %#v", err, err)
	}
}

func TestCustomRetryStatuses(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.MaxRetries = 1
	policy.HTTPStatuses = map[int]bool{409: true}
	var calls int
	client := newTestClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(409, strconv.Quote("conflict")), nil
		}
		return response(200, `{"models":[]}`), nil
	}, WithRetryPolicy(policy))
	noWait(client)
	if _, err := client.Models.List(context.Background()); err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
