package typesafe

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorForStatus(t *testing.T) {
	tests := []struct {
		status int
		check  func(error) bool
	}{
		{400, func(err error) bool { var e *BadRequestError; return errors.As(err, &e) }},
		{401, func(err error) bool { var e *AuthenticationError; return errors.As(err, &e) }},
		{403, func(err error) bool { var e *PermissionDeniedError; return errors.As(err, &e) }},
		{404, func(err error) bool { var e *NotFoundError; return errors.As(err, &e) }},
		{422, func(err error) bool { var e *UnprocessableEntityError; return errors.As(err, &e) }},
		{429, func(err error) bool { var e *RateLimitError; return errors.As(err, &e) }},
		{503, func(err error) bool { var e *InternalServerError; return errors.As(err, &e) }},
	}
	for _, tt := range tests {
		err := APIErrorForStatus(tt.status, map[string]any{"error": map[string]any{"message": "boom"}}, http.Header{"X-Typesafe-Request-Id": {"req_1"}})
		var apiErr *APIError
		if !tt.check(err) || !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "boom") || apiErr.RequestID != "req_1" {
			t.Errorf("status %d: %#v", tt.status, err)
		}
	}
}

func TestValidationErrorMessage(t *testing.T) {
	body := map[string]any{"detail": []any{
		map[string]any{"loc": []any{"body", "questions", "q"}, "msg": "invalid"},
		map[string]any{"loc": []any{"body", "state"}, "msg": "required"},
	}}
	err := APIErrorForStatus(422, body, nil)
	if want := "422 questions.q: invalid; state: required"; err.Error() != want {
		t.Fatalf("got %q, want %q", err, want)
	}
}

func TestErrorMessageFallbacks(t *testing.T) {
	tests := []struct {
		body     any
		contains string
	}{
		{map[string]any{"error": "plain"}, "plain"},
		{map[string]any{"message": "message"}, "message"},
		{map[string]any{"detail": "detail"}, "detail"},
		{map[string]any{"detail": map[string]any{"message": "nested"}}, "nested"},
		{map[string]any{"unknown": strings.Repeat("x", 250)}, "…"},
		{map[string]any{"unknown": true}, `{"unknown":true}`},
	}
	for _, tt := range tests {
		if got := APIErrorForStatus(409, tt.body, nil).Error(); !strings.Contains(got, tt.contains) {
			t.Errorf("%#v: %q", tt.body, got)
		}
	}
	if got := (&APIConnectionError{}).Error(); got != "Connection error." {
		t.Fatalf("got %q", got)
	}
	if got := (&APITimeoutError{APIConnectionError: &APIConnectionError{}, Timeout: time.Microsecond}).Error(); !strings.Contains(got, "1µs") {
		t.Fatalf("got %q", got)
	}
	abort := &APIUserAbortError{Cause: context.Canceled}
	if abort.Error() != "Request was aborted." || !errors.Is(abort, context.Canceled) {
		t.Fatal("abort error contract")
	}
	typed := &TypeSafeError{Message: "outer", Cause: context.Canceled}
	if !errors.Is(typed, context.Canceled) {
		t.Fatal("typed error did not unwrap")
	}
}
