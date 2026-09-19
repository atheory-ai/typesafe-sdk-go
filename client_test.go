package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testHTTPClient(fn roundTripFunc) *http.Client { return &http.Client{Transport: fn} }

func response(status int, body string, headers ...http.Header) *http.Response {
	header := make(http.Header)
	if len(headers) > 0 {
		header = headers[0]
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func newTestClient(t *testing.T, fn roundTripFunc, options ...Option) *Client {
	t.Helper()
	base := []Option{WithAPIKey("secret"), WithHTTPClient(testHTTPClient(fn)), WithMaxRetries(0)}
	client, err := NewClient(append(base, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientConfiguration(t *testing.T) {
	for _, name := range []string{EnvAPIKey, EnvBaseURL, EnvDefaultModel, EnvLogLevel} {
		t.Setenv(name, "")
	}
	if _, err := NewClient(); err == nil || !strings.Contains(err.Error(), EnvAPIKey) {
		t.Fatalf("missing key error: %v", err)
	}
	t.Setenv(EnvAPIKey, " env-key ")
	t.Setenv(EnvBaseURL, "https://env.test///")
	t.Setenv(EnvDefaultModel, "env-model")
	t.Setenv(EnvLogLevel, "debug")
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.BaseURL != "https://env.test" || client.DefaultModel != "env-model" || client.LogLevel != LogDebug {
		t.Fatalf("environment not resolved: %#v", client)
	}
	client, err = NewClient(WithAPIKey("code-key"), WithBaseURL("https://code.test/"), WithDefaultModel("code-model"), WithLogLevel(LogError))
	if err != nil {
		t.Fatal(err)
	}
	if client.BaseURL != "https://code.test" || client.DefaultModel != "code-model" || client.LogLevel != LogError {
		t.Fatalf("options did not win: %#v", client)
	}
	if strings.Contains(clientString(client), "code-key") {
		t.Fatal("client representation leaked API key")
	}
	client, err = NewClient(WithAPIKey(""), WithBaseURL(""), WithDefaultModel(""), WithLogger(nil))
	if err != nil || client.BaseURL != "" || client.DefaultModel != "" {
		t.Fatalf("explicit empty values did not take precedence: %#v, %v", client, err)
	}
}

func clientString(client *Client) string {
	data, _ := json.Marshal(client)
	return string(data)
}

func TestInvalidConfiguration(t *testing.T) {
	t.Setenv(EnvLogLevel, "loud")
	if _, err := NewClient(WithAPIKey("k")); err == nil || !strings.Contains(err.Error(), `"loud" from TYPESAFE_LOG_LEVEL`) {
		t.Fatalf("invalid env log level: %v", err)
	}
	t.Setenv(EnvLogLevel, "")
	if _, err := NewClient(WithAPIKey("k"), WithLogLevel(LogLevel("loud"))); err == nil || !strings.Contains(err.Error(), "the `logLevel` option") {
		t.Fatalf("invalid option log level: %v", err)
	}
	if _, err := NewClient(WithAPIKey("k"), WithTimeout(0)); err == nil {
		t.Fatal("accepted zero timeout")
	}
	if _, err := NewClient(WithAPIKey("k"), WithMaxRetries(-1)); err == nil {
		t.Fatal("accepted negative retries")
	}
	if _, err := NewClient(WithAPIKey("k"), WithHTTPClient(nil)); err == nil {
		t.Fatal("accepted nil HTTP client")
	}
}

func TestModelsListHeadersAndRawResponse(t *testing.T) {
	var got *http.Request
	client := newTestClient(t, func(request *http.Request) (*http.Response, error) {
		got = request.Clone(request.Context())
		return response(200, `{"models":[{"name":"m","description":"d","release_date":"2026","internal":true}]}`, http.Header{
			"Content-Type": {"application/json"}, "X-Typesafe-Request-Id": {"req_1"},
		}), nil
	}, WithBaseURL("https://x.test/"), WithDefaultHeaders(http.Header{"X-Trace": {"default"}, "Authorization": {"bad"}}))
	result, err := client.Models.ListWithResponse(context.Background(), WithRequestHeaders(http.Header{"X-Trace": {"call"}}))
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.String() != "https://x.test/v1/models" || got.Method != http.MethodGet {
		t.Fatalf("request: %s %s", got.Method, got.URL)
	}
	if got.Header.Get("Authorization") != "Bearer secret" || got.Header.Get("X-Trace") != "call" {
		t.Fatalf("headers: %v", got.Header)
	}
	if got.Header.Get("Content-Type") != "" || got.Header.Get("X-TypeSafe-SDK") != "typesafe-sdk/"+Version {
		t.Fatalf("SDK headers: %v", got.Header)
	}
	if !strings.HasPrefix(got.Header.Get("X-TypeSafe-Runtime"), "go/") {
		t.Fatalf("runtime: %q", got.Header.Get("X-TypeSafe-Runtime"))
	}
	if len(result.Data) != 1 || result.Data[0].Name != "m" || result.RequestID != "req_1" {
		t.Fatalf("result: %#v", result)
	}
	raw, err := io.ReadAll(result.Response.Body)
	if err != nil || !strings.Contains(string(raw), `"internal":true`) {
		t.Fatalf("raw body: %q, %v", raw, err)
	}
}

func TestModelsListRejectsMalformedShape(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{}`, `{"models":null}`, `{"models":{}}`, `{"models":"bad"}`} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
			_, err := client.Models.List(context.Background())
			if err == nil || !strings.Contains(err.Error(), "Unexpected response shape") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSystemOnePayloadAndAnswers(t *testing.T) {
	var payload map[string]any
	client := newTestClient(t, func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return response(200, `{"model":"m","answers":{"q":{"type":"choice","choice":"b","confidence":0.9,"probabilities":{"a":0.1,"b":0.9}}},"usage":{"input_tokens":1,"output_tokens":1}}`), nil
	}, WithDefaultModel("client-default"))
	result, err := client.SystemOne(context.Background(), SystemOneRequest{
		State: map[string]any{"a": 1}, Questions: Questions{"q": Choice("pick", map[string]any{"a": nil, "b": nil})},
		Extra: map[string]any{"future_option": nil, "nested": map[string]any{"enabled": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "client-default" || payload["future_option"] != nil {
		t.Fatalf("payload: %#v", payload)
	}
	answer, ok := result.Answers["q"].(*ChoiceResponse)
	if !ok || answer.Choice != "b" {
		t.Fatalf("answer: %#v", result.Answers["q"])
	}
}

func TestSystemOneValidatesBeforeSending(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return response(200, `{}`), nil })
	_, err := client.SystemOne(context.Background(), SystemOneRequest{State: "s", Questions: Questions{}})
	if err == nil || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestHTTPErrorBodies(t *testing.T) {
	tests := []struct{ body, contentType, want string }{
		{`{"error":{"message":"bad key"}}`, "application/json", "401 bad key"},
		{`{"detail":[{"loc":["body","questions","q"],"msg":"invalid"}]}`, "", "401 questions.q: invalid"},
		{"plain failure", "text/plain", "401 plain failure"},
		{"", "application/json", "401 status code (no body)"},
	}
	for _, tt := range tests {
		client := newTestClient(t, func(*http.Request) (*http.Response, error) {
			return response(401, tt.body, http.Header{"Content-Type": {tt.contentType}}), nil
		})
		_, err := client.Models.List(context.Background())
		var auth *AuthenticationError
		if !errors.As(err, &auth) || err.Error() != tt.want {
			t.Errorf("body %q: %T %v", tt.body, err, err)
		}
	}
}

func TestConnectionError(t *testing.T) {
	boom := errors.New("fetch failed")
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return nil, boom })
	_, err := client.Models.List(context.Background())
	var connection *APIConnectionError
	if !errors.As(err, &connection) || !errors.Is(err, boom) {
		t.Fatalf("got %T %v", err, err)
	}
}

func TestEnvironmentIsTrimmed(t *testing.T) {
	old, existed := os.LookupEnv(EnvAPIKey)
	t.Cleanup(func() {
		if existed {
			os.Setenv(EnvAPIKey, old)
		} else {
			os.Unsetenv(EnvAPIKey)
		}
	})
	os.Setenv(EnvAPIKey, "   ")
	if _, err := NewClient(); err == nil {
		t.Fatal("blank key accepted")
	}
}

func TestConcurrentRequestNumbersAreSafe(t *testing.T) {
	client := newTestClient(t, func(*http.Request) (*http.Response, error) { return response(200, `{"models":[]}`), nil })
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = client.Models.List(context.Background()) }()
	}
	wg.Wait()
	if got := client.requestCount.Load(); got != 10 {
		t.Fatalf("request count %d", got)
	}
}
