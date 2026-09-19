# TypeSafe AI SDK for Go

An idiomatic Go client for the [TypeSafe AI](https://typesafe.ai) System One API.

> [!IMPORTANT]
> This is an independent, **unofficial community port**. It is not maintained,
> sponsored, endorsed, or supported by TypeSafe. For official SDKs, use the
> [JavaScript/TypeScript SDK](https://github.com/typesafe-ai/typesafe-sdk-js) or
> the [Python SDK](https://github.com/typesafe-ai/typesafe-sdk-python).

This port tracks the behavior and wire format of the official JavaScript SDK
[`v0.6.0`](https://github.com/typesafe-ai/typesafe-sdk-js/tree/v0.6.0). It has
live-tested support for all three question primitives, model discovery,
retries, timeouts, cancellation, structured errors, logging, and raw response
access.

## Requirements

- Go 1.24 or newer
- A TypeSafe API key

## Installation

```sh
go get github.com/atheory-ai/typesafe-sdk-go
```

## Quickstart

Set your API key in the environment:

```sh
export TYPESAFE_API_KEY="your-api-key"
```

Then ask one or more named questions about the same state:

```go
package main

import (
	"context"
	"fmt"
	"log"

	typesafe "github.com/atheory-ai/typesafe-sdk-go"
)

func main() {
	client, err := typesafe.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State: map[string]any{
			"document": "I was charged twice. Please fix this ASAP.",
		},
		Questions: typesafe.Questions{
			"category": typesafe.Choice("What is this ticket about?", map[string]any{
				"billing":   nil,
				"technical": nil,
				"other":     nil,
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	category := result.Answers["category"].(*typesafe.ChoiceResponse)
	fmt.Println(category.Choice)
}
```

## Question primitives

Every request supplies a state and a non-empty set of named questions. All
questions are evaluated against the same state.

```go
questions := typesafe.Questions{
	// A probability from 0 to 1.
	"urgent": typesafe.Noul("Does this require immediate attention?"),

	// One label, its confidence, and probabilities for every label.
	"team": typesafe.Choice("Which team owns this?", map[string]any{
		"billing": "Payments, invoices, and refunds",
		"support": "Product and technical help",
	}),

	// An expected score, confidence, legend, and score probabilities.
	"severity": typesafe.Score("How severe is this?", []any{
		"Minor inconvenience",
		"Important but work can continue",
		"Work is blocked",
	}),
}
```

Answers implement `typesafe.Answer` and decode to `*NoulResponse`,
`*ChoiceResponse`, or `*ScoreResponse`. Use a type assertion when the question
is known, or a type switch for heterogeneous questions:

```go
for name, answer := range result.Answers {
	switch answer := answer.(type) {
	case *typesafe.NoulResponse:
		fmt.Println(name, answer.Noul)
	case *typesafe.ChoiceResponse:
		fmt.Println(name, answer.Choice, answer.Confidence)
	case *typesafe.ScoreResponse:
		fmt.Println(name, answer.Score, answer.Confidence)
	}
}
```

Instructions, state, and criteria may contain any JSON-compatible value. Use
`nil` for an intentionally undescribed criterion.

## Listing models

```go
models, err := client.Models.List(context.Background())
if err != nil {
	log.Fatal(err)
}
for _, model := range models {
	fmt.Println(model.Name, model.Description, model.ReleaseDate)
}
```

## Configuration

Explicit options take precedence over environment variables, which take
precedence over SDK defaults.

| Setting | Client option | Environment | Default |
| --- | --- | --- | --- |
| API key | `WithAPIKey` | `TYPESAFE_API_KEY` | required |
| Base URL | `WithBaseURL` | `TYPESAFE_BASE_URL` | `https://api.typesafe.ai` |
| Model | `WithDefaultModel` | `TYPESAFE_DEFAULT_MODEL` | `jev-latest` |
| Log level | `WithLogLevel` | `TYPESAFE_LOG_LEVEL` | `warn` |
| Attempt timeout | `WithTimeout` | — | 10 seconds |
| Retry policy | `WithRetryPolicy` | — | see below |
| Maximum retries | `WithMaxRetries` | — | 2 |
| HTTP transport | `WithHTTPClient` | — | `http.DefaultClient` |

```go
client, err := typesafe.NewClient(
	typesafe.WithAPIKey("..."),
	typesafe.WithDefaultModel("jev-latest"),
	typesafe.WithTimeout(20*time.Second),
	typesafe.WithLogLevel(typesafe.LogInfo),
)
```

Per-call overrides are available through `WithRequestTimeout`,
`WithRequestRetryPolicy`, `WithRequestMaxRetries`, and `WithRequestHeaders`.
Supply a custom `http.Client` to configure proxies, TLS, tracing, or another
transport.

## Retries and timeouts

The default policy retries HTTP 408, 429, and 5xx responses, connection errors,
and timeouts. It makes at most two retries after the initial attempt, uses
capped exponential backoff from 500 milliseconds to 5 seconds with 25% jitter,
and honors `Retry-After` or `retry-after-ms` values up to one minute.

Timeouts apply independently to every attempt and cover delivery of the full
response body. Cancel the supplied `context.Context` to stop an in-flight
request or a pending retry.

To customize the policy, start from the defaults so unspecified behavior is
preserved:

```go
policy := typesafe.DefaultRetryPolicy()
policy.MaxRetries = 4
policy.BackoffMax = 10 * time.Second

client, err := typesafe.NewClient(typesafe.WithRetryPolicy(policy))
```

## Error handling

All errors follow normal Go wrapping conventions and support `errors.Is` and
`errors.As`.

```go
result, err := client.SystemOne(ctx, request)
if err != nil {
	var rateLimit *typesafe.RateLimitError
	if errors.As(err, &rateLimit) && rateLimit.HasRetryAfter {
		fmt.Println("retry after", rateLimit.RetryAfter)
	}
	return err
}
```

HTTP errors are classified as `BadRequestError`, `AuthenticationError`,
`PermissionDeniedError`, `NotFoundError`, `UnprocessableEntityError`,
`RateLimitError`, or `InternalServerError`. Every HTTP error retains its status,
headers, parsed body, and request ID. Transport failures use
`APIConnectionError`, `APITimeoutError`, and `APIUserAbortError`.

## Raw response access

Use `SystemOneWithResponse` or `client.Models.ListWithResponse` to receive the
decoded value together with the buffered `*http.Response` and
`x-typesafe-request-id`:

```go
response, err := client.SystemOneWithResponse(ctx, request)
if err != nil {
	return err
}
fmt.Println(response.RequestID, response.Response.StatusCode)
```

The raw response body remains readable after decoding.

## Security

Treat the API key as a server-side secret. Do not embed it in browser,
desktop, mobile, or other distributable client binaries. The repository ignores
`.env`, `.env.*`, `.envrc`, and `.direnv/`; `.env.example` is the only intended
environment file to commit.

If a real key is ever committed, removing the file from the latest commit is
not sufficient—revoke the key and remove it from Git history.

## Compatibility and project status

The compatibility baseline is the official JavaScript SDK `v0.6.0` at commit
`66880ccded6cb642dc1809620c2b108c33730214`. The offline conformance suite maps
the upstream behavior for wire formats, configuration, errors, logging,
retries, timeouts, cancellation, response buffering, and model discovery.
Credential-gated tests also run against the live TypeSafe API.

See [UPSTREAM.md](UPSTREAM.md) for the detailed parity map. Because this is an
unofficial port, upstream API or SDK changes may appear here later than in the
official clients.

## Development

Run the offline checks:

```sh
gofmt -w .
go vet ./...
go test -race -cover ./...
```

To include live API conformance tests:

```sh
cp .env.example .env
# Add TYPESAFE_API_KEY to .env, then:
set -a; . ./.env; set +a
go test -race -count=1 ./...
```

CI enforces formatting, vetting, race safety, and a 90% statement-coverage
floor. The current suite is above 94% without requiring live credentials.

## Attribution

This project is derived from the MIT-licensed official
[TypeSafe JavaScript SDK](https://github.com/typesafe-ai/typesafe-sdk-js).
TypeSafe and its official SDK maintainers are not responsible for this port.
See [NOTICE](NOTICE) and [LICENSE](LICENSE).

## License

MIT
