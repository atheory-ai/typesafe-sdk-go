# Upstream parity

This port is based on `typesafe-ai/typesafe-sdk-js` version `v0.6.0`, commit:

```text
66880ccded6cb642dc1809620c2b108c33730214
```

The Go conformance suite covers the corresponding public behavior:

| Upstream area | Go coverage |
| --- | --- |
| Question builders and JSON wire format | `questions_test.go` |
| Client configuration and environment precedence | `client_test.go` |
| System One and models resources | `client_test.go` |
| Status-specific errors and readable server validation errors | `errors_test.go`, `client_test.go` |
| Retry defaults, `Retry-After`, backoff, and policy overrides | `retry_test.go`, `reliability_test.go` |
| Per-attempt timeouts and caller cancellation | `reliability_test.go` |
| Protected headers and buffered raw responses | `client_test.go` |
| Log filtering and credential redaction | `logging_test.go` |

Language-specific adaptations:

- `context.Context` replaces `AbortSignal`.
- Functional options replace JavaScript configuration objects.
- `SystemOneWithResponse` and `Models.ListWithResponse` replace promise helpers.
- `errors.As` replaces JavaScript `instanceof` checks.
- Heterogeneous answers decode through the `Answer` interface and concrete response structs.

Integration tests against the live API require credentials and are intentionally not run by the offline unit suite.
