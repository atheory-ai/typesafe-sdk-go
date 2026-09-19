package typesafe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type TypeSafeError struct {
	Message string
	Cause   error
}

func (e *TypeSafeError) Error() string { return e.Message }
func (e *TypeSafeError) Unwrap() error { return e.Cause }

// APIError represents an unsuccessful HTTP response.
type APIError struct {
	Status    int
	Header    http.Header
	Body      any
	RequestID string
	Message   string
}

func (e *APIError) Error() string { return e.Message }

type BadRequestError struct{ *APIError }
type AuthenticationError struct{ *APIError }
type PermissionDeniedError struct{ *APIError }
type NotFoundError struct{ *APIError }
type UnprocessableEntityError struct{ *APIError }
type InternalServerError struct{ *APIError }

func (e *BadRequestError) Unwrap() error          { return e.APIError }
func (e *AuthenticationError) Unwrap() error      { return e.APIError }
func (e *PermissionDeniedError) Unwrap() error    { return e.APIError }
func (e *NotFoundError) Unwrap() error            { return e.APIError }
func (e *UnprocessableEntityError) Unwrap() error { return e.APIError }
func (e *InternalServerError) Unwrap() error      { return e.APIError }

type RateLimitError struct {
	*APIError
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *RateLimitError) Unwrap() error { return e.APIError }

// APIConnectionError wraps DNS, TLS, socket, and response delivery failures.
type APIConnectionError struct {
	Message string
	Cause   error
}

func (e *APIConnectionError) Error() string {
	if e.Message == "" {
		return "Connection error."
	}
	return e.Message
}
func (e *APIConnectionError) Unwrap() error { return e.Cause }

// APITimeoutError indicates that an attempt exceeded its configured timeout.
type APITimeoutError struct {
	*APIConnectionError
	Timeout time.Duration
}

func (e *APITimeoutError) Error() string {
	return fmt.Sprintf("Request timed out after %s.", formatDurationMS(e.Timeout))
}
func (e *APITimeoutError) Unwrap() error { return e.APIConnectionError }

// APIUserAbortError indicates cancellation through the caller's context.
type APIUserAbortError struct{ Cause error }

func (e *APIUserAbortError) Error() string { return "Request was aborted." }
func (e *APIUserAbortError) Unwrap() error { return e.Cause }

func formatDurationMS(d time.Duration) string {
	if d%time.Millisecond == 0 {
		return fmt.Sprintf("%dms", d/time.Millisecond)
	}
	return d.String()
}

// APIErrorForStatus selects the documented error subtype for a status code.
func APIErrorForStatus(status int, body any, header http.Header) error {
	if header == nil {
		header = make(http.Header)
	}
	base := &APIError{
		Status: status, Header: header.Clone(), Body: body,
		RequestID: header.Get(RequestIDHeader), Message: describeAPIError(status, body),
	}
	switch {
	case status == 400:
		return &BadRequestError{base}
	case status == 401:
		return &AuthenticationError{base}
	case status == 403:
		return &PermissionDeniedError{base}
	case status == 404:
		return &NotFoundError{base}
	case status == 422:
		return &UnprocessableEntityError{base}
	case status == 429:
		delay, ok := ParseRetryAfter(header, time.Now())
		return &RateLimitError{APIError: base, RetryAfter: delay, HasRetryAfter: ok}
	case status >= 500:
		return &InternalServerError{base}
	default:
		return base
	}
}

func describeAPIError(status int, body any) string {
	if detail := extractMessage(body); detail != "" {
		return fmt.Sprintf("%d %s", status, detail)
	}
	if body == nil {
		return fmt.Sprintf("%d status code (no body)", status)
	}
	var raw string
	if text, ok := body.(string); ok {
		raw = text
	} else if data, err := json.Marshal(body); err == nil {
		raw = string(data)
	} else {
		raw = fmt.Sprint(body)
	}
	if len(raw) > 200 {
		raw = raw[:200] + "…"
	}
	return fmt.Sprintf("%d %s", status, raw)
}

func extractMessage(body any) string {
	if text, ok := body.(string); ok {
		return text
	}
	record, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	if text, ok := record["error"].(string); ok {
		return text
	}
	if nested, ok := record["error"].(map[string]any); ok {
		if text, ok := nested["message"].(string); ok {
			return text
		}
	}
	if text, ok := record["message"].(string); ok {
		return text
	}
	if text, ok := record["detail"].(string); ok {
		return text
	}
	if nested, ok := record["detail"].(map[string]any); ok {
		if text, ok := nested["message"].(string); ok {
			return text
		}
	}
	if entries, ok := record["detail"].([]any); ok {
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			item, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			msg, ok := item["msg"].(string)
			if !ok {
				continue
			}
			path := make([]string, 0)
			if loc, ok := item["loc"].([]any); ok {
				for _, segment := range loc {
					value := fmt.Sprint(segment)
					if value != "body" {
						path = append(path, value)
					}
				}
			}
			if len(path) > 0 {
				msg = strings.Join(path, ".") + ": " + msg
			}
			parts = append(parts, msg)
		}
		return strings.Join(parts, "; ")
	}
	return ""
}
