package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Response contains decoded data and the buffered HTTP response. Response.Body
// remains readable after the method returns.
type Response[T any] struct {
	Data      T
	Response  *http.Response
	RequestID string
}

type Client struct {
	BaseURL        string
	DefaultModel   string
	LogLevel       LogLevel
	Logger         Logger
	Retry          RetryPolicy
	Timeout        time.Duration
	DefaultHeaders http.Header
	HTTPClient     *http.Client
	Models         *Models

	apiKey       string
	random       func() float64
	sleep        func(context.Context, time.Duration) error
	requestCount atomic.Uint64
}

// NewClient constructs a client. Explicit options take precedence over
// environment values, which take precedence over SDK defaults.
func NewClient(options ...Option) (*Client, error) {
	cfg := clientConfig{
		retry: DefaultRetryPolicy(), timeout: DefaultTimeout, headers: make(http.Header),
		httpClient: http.DefaultClient, logger: defaultLogger{}, random: rand.Float64,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option.apply(&cfg); err != nil {
			return nil, err
		}
	}
	apiKey, apiKeyFound := valueFromOptionOrEnv(cfg.apiKey, cfg.apiKeySet, EnvAPIKey)
	if !apiKeyFound {
		return nil, &TypeSafeError{Message: "No API key was provided. Pass WithAPIKey to NewClient or set the " + EnvAPIKey + " environment variable."}
	}
	baseURL, baseURLFound := valueFromOptionOrEnv(cfg.baseURL, cfg.baseURLSet, EnvBaseURL)
	if !baseURLFound {
		baseURL = DefaultBaseURL
	}
	model, modelFound := valueFromOptionOrEnv(cfg.model, cfg.modelSet, EnvDefaultModel)
	if !modelFound {
		model = DefaultModel
	}
	logValue, logLevelFound := valueFromOptionOrEnv(cfg.logLevel, cfg.logLevelSet, EnvLogLevel)
	if !logLevelFound {
		logValue = string(LogWarn)
	}
	level, err := parseLogLevel(logValue, logSource(cfg.logLevelSet))
	if err != nil {
		return nil, err
	}
	if cfg.httpClient == nil {
		return nil, &TypeSafeError{Message: "HTTP client cannot be nil."}
	}
	if cfg.logger == nil {
		cfg.logger = defaultLogger{}
	}
	if err := validateRetryPolicy(cfg.retry); err != nil {
		return nil, err
	}
	client := &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), DefaultModel: model,
		LogLevel: level, Logger: levelLogger{sink: cfg.logger, level: level},
		Retry: cfg.retry.clone(), Timeout: cfg.timeout, DefaultHeaders: cfg.headers.Clone(),
		HTTPClient: cfg.httpClient, apiKey: apiKey, random: cfg.random,
	}
	client.sleep = sleepContext
	client.Models = &Models{client: client}
	return client, nil
}

func valueFromOptionOrEnv(value string, set bool, name string) (string, bool) {
	if set {
		return value, true
	}
	value = strings.TrimSpace(os.Getenv(name))
	return value, value != ""
}

func logSource(explicit bool) string {
	if explicit {
		return "the `logLevel` option"
	}
	return EnvLogLevel
}

// SystemOne answers named questions about text or structured state.
func (c *Client) SystemOne(ctx context.Context, request SystemOneRequest, options ...RequestOption) (*SystemOneResult, error) {
	response, err := c.SystemOneWithResponse(ctx, request, options...)
	if err != nil {
		return nil, err
	}
	return &response.Data, nil
}

// SystemOneWithResponse also returns the buffered HTTP response and request ID.
func (c *Client) SystemOneWithResponse(ctx context.Context, request SystemOneRequest, options ...RequestOption) (*Response[SystemOneResult], error) {
	if err := ValidateQuestions(request.Questions); err != nil {
		return nil, err
	}
	return doJSON[SystemOneResult](c, ctx, http.MethodPost, "/v1/systemone", request.payload(c.DefaultModel), options...)
}

type Models struct{ client *Client }

func (m *Models) List(ctx context.Context, options ...RequestOption) ([]ModelCard, error) {
	response, err := m.ListWithResponse(ctx, options...)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (m *Models) ListWithResponse(ctx context.Context, options ...RequestOption) (*Response[[]ModelCard], error) {
	wire, err := doJSON[modelsWire](m.client, ctx, http.MethodGet, "/v1/models", nil, options...)
	if err != nil {
		return nil, err
	}
	models, err := wire.Data.unwrap()
	if err != nil {
		return nil, err
	}
	return &Response[[]ModelCard]{Data: models, Response: wire.Response, RequestID: wire.RequestID}, nil
}

type modelsWire struct {
	Models json.RawMessage `json:"models"`
}

func (w *modelsWire) UnmarshalJSON(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		w.Models = nil
		return nil
	}
	w.Models = object["models"]
	return nil
}

func (w modelsWire) unwrap() ([]ModelCard, error) {
	raw := bytes.TrimSpace(w.Models)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, &TypeSafeError{Message: "Unexpected response shape from GET /v1/models; expected { models: [...] }."}
	}
	var cards []ModelCard
	if err := json.Unmarshal(raw, &cards); err != nil {
		return nil, &TypeSafeError{Message: "Unexpected response shape from GET /v1/models; expected { models: [...] }.", Cause: err}
	}
	return cards, nil
}

func doJSON[T any](c *Client, ctx context.Context, method, path string, body any, options ...RequestOption) (*Response[T], error) {
	resolved := requestOptions{timeout: c.Timeout, retry: c.Retry.clone(), headers: make(http.Header)}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option.applyRequest(&resolved); err != nil {
			return nil, err
		}
	}
	data, response, err := c.fetchWithRetries(ctx, method, path, body, resolved)
	if err != nil {
		return nil, err
	}
	var decoded T
	if len(data) > 0 {
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, &TypeSafeError{Message: "Failed to decode API response: " + err.Error(), Cause: err}
		}
	}
	c.Logger.Debug("response body: %s", string(data))
	return &Response[T]{Data: decoded, Response: response, RequestID: response.Header.Get(RequestIDHeader)}, nil
}

func (c *Client) fetchWithRetries(ctx context.Context, method, path string, body any, options requestOptions) ([]byte, *http.Response, error) {
	url := c.BaseURL + path
	headers := mergeHeaders(c.DefaultHeaders, options.headers)
	headers.Set("Authorization", "Bearer "+c.apiKey)
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "typesafe-sdk/"+Version)
	headers.Set("X-TypeSafe-SDK", "typesafe-sdk/"+Version)
	headers.Set("X-TypeSafe-Runtime", describeRuntime())
	headers.Del("X-TypeSafe-Retry-Count")
	var encoded []byte
	var err error
	if body != nil {
		headers.Set("Content-Type", "application/json")
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, nil, &TypeSafeError{Message: "Failed to encode request: " + err.Error(), Cause: err}
		}
	} else {
		headers.Del("Content-Type")
	}
	tag := fmt.Sprintf("#%d %s %s", c.requestCount.Add(1), method, path)
	for attempt := 0; ; attempt++ {
		attemptHeaders := headers.Clone()
		if attempt > 0 {
			attemptHeaders.Set("X-TypeSafe-Retry-Count", fmt.Sprint(attempt))
		}
		c.Logger.Debug("%s -> %s headers=%v body=%s", tag, url, RedactHeaders(headerStrings(attemptHeaders)), string(encoded))
		started := time.Now()
		data, response, requestErr := c.attempt(ctx, method, url, attemptHeaders, encoded, options.timeout)
		if requestErr != nil {
			if _, aborted := requestErr.(*APIUserAbortError); aborted {
				return nil, nil, requestErr
			}
			if attempt >= options.retry.MaxRetries || !retryableError(requestErr, options.retry) {
				return nil, nil, requestErr
			}
			if err := c.backoff(ctx, tag, attempt, options.retry, nil, requestErr.Error()); err != nil {
				return nil, nil, err
			}
			continue
		}
		requestID := response.Header.Get(RequestIDHeader)
		suffix := ""
		if requestID != "" {
			suffix = " (request " + requestID + ")"
		}
		c.Logger.Info("%s <- %d in %s%s", tag, response.StatusCode, time.Since(started).Round(time.Millisecond), suffix)
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return data, response, nil
		}
		parsed := parseBody(data, response.Header)
		apiErr := APIErrorForStatus(response.StatusCode, parsed, response.Header)
		if attempt >= options.retry.MaxRetries || !options.retry.HTTPStatuses[response.StatusCode] {
			return nil, nil, apiErr
		}
		if err := c.backoff(ctx, tag, attempt, options.retry, response.Header, fmt.Sprint(response.StatusCode)); err != nil {
			return nil, nil, err
		}
	}
}

func (c *Client) attempt(parent context.Context, method, url string, headers http.Header, body []byte, timeout time.Duration) ([]byte, *http.Response, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, &TypeSafeError{Message: "Failed to create request: " + err.Error(), Cause: err}
	}
	request.Header = headers
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return nil, nil, &APIUserAbortError{Cause: err}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, &APITimeoutError{APIConnectionError: &APIConnectionError{Cause: err}, Timeout: timeout}
		}
		return nil, nil, &APIConnectionError{Message: "Connection error: " + err.Error(), Cause: err}
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(data))
	if readErr != nil {
		if parent.Err() != nil {
			return nil, nil, &APIUserAbortError{Cause: readErr}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, &APITimeoutError{APIConnectionError: &APIConnectionError{Cause: readErr}, Timeout: timeout}
		}
		return nil, nil, &APIConnectionError{Message: "Connection error: " + readErr.Error(), Cause: readErr}
	}
	if parent.Err() != nil {
		return nil, nil, &APIUserAbortError{Cause: parent.Err()}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, nil, &APITimeoutError{APIConnectionError: &APIConnectionError{Cause: ctx.Err()}, Timeout: timeout}
	}
	return data, response, nil
}

func (c *Client) backoff(ctx context.Context, tag string, attempt int, policy RetryPolicy, header http.Header, reason string) error {
	delay := RetryDelay(attempt, header, policy, c.random)
	c.Logger.Info("%s retrying in %s (retry %d/%d) after %s", tag, delay, attempt+1, policy.MaxRetries, reason)
	if err := c.sleep(ctx, delay); err != nil {
		return &APIUserAbortError{Cause: err}
	}
	return nil
}

func retryableError(err error, policy RetryPolicy) bool {
	var timeout *APITimeoutError
	if errors.As(err, &timeout) {
		return policy.APITimeoutError
	}
	var connection *APIConnectionError
	return errors.As(err, &connection) && policy.APIConnectionError
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func mergeHeaders(sources ...http.Header) http.Header {
	result := make(http.Header)
	for _, source := range sources {
		for name, values := range source {
			result.Del(name)
			for _, value := range values {
				result.Add(name, value)
			}
		}
	}
	return result
}

func headerStrings(header http.Header) map[string]string {
	result := make(map[string]string, len(header))
	for name, values := range header {
		result[name] = strings.Join(values, ", ")
	}
	return result
}

func describeRuntime() string {
	return fmt.Sprintf("go/%s (%s; %s)", strings.TrimPrefix(runtime.Version(), "go"), runtime.GOOS, runtime.GOARCH)
}

func parseBody(data []byte, header http.Header) any {
	if len(data) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		return value
	}
	return string(data)
}
