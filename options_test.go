package typesafe

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestOptions(t *testing.T) {
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(200, `{"models":[]}`), nil })
	policy := DefaultRetryPolicy()
	policy.MaxRetries = 0
	if _, err := client.Models.List(context.Background(), WithRequestTimeout(time.Second), WithRequestRetryPolicy(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Models.List(context.Background(), WithRequestTimeout(0)); err == nil {
		t.Fatal("accepted zero request timeout")
	}
	if _, err := client.Models.List(context.Background(), WithRequestMaxRetries(-1)); err == nil {
		t.Fatal("accepted negative request retries")
	}
	bad := DefaultRetryPolicy()
	bad.BackoffJitter = 2
	if _, err := client.Models.List(context.Background(), WithRequestRetryPolicy(bad)); err == nil {
		t.Fatal("accepted bad request policy")
	}
}

func TestRetryPolicyValidationFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RetryPolicy)
		want   string
	}{
		{"initial", func(p *RetryPolicy) { p.BackoffInitial = -1 }, "backoffInitial"},
		{"max", func(p *RetryPolicy) { p.BackoffMax = -1 }, "backoffMax"},
		{"jitter", func(p *RetryPolicy) { p.BackoffJitter = -1 }, "backoffJitter"},
		{"retry-after", func(p *RetryPolicy) { p.MaxRetryAfter = -1 }, "maxRetryAfter"},
		{"status", func(p *RetryPolicy) { p.HTTPStatuses = map[int]bool{99: true} }, "httpStatuses"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := DefaultRetryPolicy()
			tt.mutate(&policy)
			_, err := NewClient(WithAPIKey("k"), WithRetryPolicy(policy))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRetryPolicyIsCopied(t *testing.T) {
	policy := DefaultRetryPolicy()
	client, err := NewClient(WithAPIKey("k"), WithRetryPolicy(policy))
	if err != nil {
		t.Fatal(err)
	}
	policy.HTTPStatuses[409] = true
	if client.Retry.HTTPStatuses[409] {
		t.Fatal("client retained caller-owned status map")
	}
	other, err := NewClient(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	client.Retry.HTTPStatuses[409] = true
	if other.Retry.HTTPStatuses[409] {
		t.Fatal("default policies share status maps")
	}
}
