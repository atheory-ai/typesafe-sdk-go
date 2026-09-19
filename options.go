package typesafe

import (
	"net/http"
	"time"
)

// Option configures a Client during construction.
type Option interface{ apply(*clientConfig) error }

type optionFunc func(*clientConfig) error

func (f optionFunc) apply(config *clientConfig) error { return f(config) }

type clientConfig struct {
	apiKeySet, baseURLSet, modelSet, logLevelSet bool
	apiKey, baseURL, model, logLevel             string
	logger                                       Logger
	retry                                        RetryPolicy
	timeout                                      time.Duration
	headers                                      http.Header
	httpClient                                   *http.Client
	random                                       func() float64
}

func WithAPIKey(value string) Option {
	return optionFunc(func(c *clientConfig) error { c.apiKey, c.apiKeySet = value, true; return nil })
}
func WithBaseURL(value string) Option {
	return optionFunc(func(c *clientConfig) error { c.baseURL, c.baseURLSet = value, true; return nil })
}
func WithDefaultModel(value string) Option {
	return optionFunc(func(c *clientConfig) error { c.model, c.modelSet = value, true; return nil })
}
func WithLogLevel(value LogLevel) Option {
	return optionFunc(func(c *clientConfig) error { c.logLevel, c.logLevelSet = string(value), true; return nil })
}
func WithLogger(value Logger) Option {
	return optionFunc(func(c *clientConfig) error { c.logger = value; return nil })
}
func WithHTTPClient(value *http.Client) Option {
	return optionFunc(func(c *clientConfig) error { c.httpClient = value; return nil })
}
func WithDefaultHeaders(value http.Header) Option {
	return optionFunc(func(c *clientConfig) error { c.headers = value.Clone(); return nil })
}
func WithTimeout(value time.Duration) Option {
	return optionFunc(func(c *clientConfig) error {
		if value <= 0 {
			return &TypeSafeError{Message: "`timeout` must be a positive duration, got " + value.String() + "."}
		}
		c.timeout = value
		return nil
	})
}
func WithRetryPolicy(value RetryPolicy) Option {
	return optionFunc(func(c *clientConfig) error {
		if err := validateRetryPolicy(value); err != nil {
			return err
		}
		c.retry = value.clone()
		return nil
	})
}
func WithMaxRetries(value int) Option {
	return optionFunc(func(c *clientConfig) error {
		if value < 0 {
			return &TypeSafeError{Message: "`retry.maxRetries` must be a non-negative integer."}
		}
		c.retry.MaxRetries = value
		return nil
	})
}

// RequestOption overrides client settings for one API call.
type RequestOption interface{ applyRequest(*requestOptions) error }

type requestOptionFunc func(*requestOptions) error

func (f requestOptionFunc) applyRequest(options *requestOptions) error { return f(options) }

type requestOptions struct {
	timeout time.Duration
	retry   RetryPolicy
	headers http.Header
}

func WithRequestTimeout(value time.Duration) RequestOption {
	return requestOptionFunc(func(o *requestOptions) error {
		if value <= 0 {
			return &TypeSafeError{Message: "`timeout` must be a positive duration, got " + value.String() + "."}
		}
		o.timeout = value
		return nil
	})
}
func WithRequestRetryPolicy(value RetryPolicy) RequestOption {
	return requestOptionFunc(func(o *requestOptions) error {
		if err := validateRetryPolicy(value); err != nil {
			return err
		}
		o.retry = value.clone()
		return nil
	})
}
func WithRequestMaxRetries(value int) RequestOption {
	return requestOptionFunc(func(o *requestOptions) error {
		if value < 0 {
			return &TypeSafeError{Message: "`retry.maxRetries` must be a non-negative integer."}
		}
		o.retry.MaxRetries = value
		return nil
	})
}
func WithRequestHeaders(value http.Header) RequestOption {
	return requestOptionFunc(func(o *requestOptions) error { o.headers = value.Clone(); return nil })
}

func validateRetryPolicy(p RetryPolicy) error {
	switch {
	case p.MaxRetries < 0:
		return &TypeSafeError{Message: "`retry.maxRetries` must be a non-negative integer."}
	case p.BackoffInitial < 0:
		return &TypeSafeError{Message: "`retry.backoffInitial` must be non-negative."}
	case p.BackoffMax < 0:
		return &TypeSafeError{Message: "`retry.backoffMax` must be non-negative."}
	case p.BackoffJitter < 0 || p.BackoffJitter > 1:
		return &TypeSafeError{Message: "`retry.backoffJitter` must be between 0 and 1."}
	case p.MaxRetryAfter < 0:
		return &TypeSafeError{Message: "`retry.maxRetryAfter` must be non-negative."}
	}
	for status := range p.HTTPStatuses {
		if status < 100 || status > 999 {
			return &TypeSafeError{Message: "`retry.httpStatuses` must contain HTTP status codes."}
		}
	}
	return nil
}
