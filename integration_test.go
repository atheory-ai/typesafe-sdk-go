package typesafe

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Live API conformance tests are skipped unless TYPESAFE_API_KEY is set.
func TestLiveAPI(t *testing.T) {
	if strings.TrimSpace(os.Getenv(EnvAPIKey)) == "" {
		t.Skip("set " + EnvAPIKey + " to run live API conformance tests")
	}
	client, err := NewClient(WithLogLevel(LogInfo), WithTimeout(120*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("models", func(t *testing.T) {
		result, err := client.Models.ListWithResponse(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(result.RequestID, "req_") || len(result.Data) == 0 {
			t.Fatalf("request ID=%q models=%d", result.RequestID, len(result.Data))
		}
		for _, model := range result.Data {
			if model.Name == "" || model.Description == "" || model.ReleaseDate == "" {
				t.Fatalf("incomplete model: %#v", model)
			}
		}
	})

	ticket := map[string]any{
		"subject": "Charged twice this month",
		"body":    "I see two charges of $49 on my card for August. I only have one account. Please fix this ASAP.",
	}
	t.Run("question primitives", func(t *testing.T) {
		result, err := client.SystemOne(ctx, SystemOneRequest{State: ticket, Questions: Questions{
			"isBilling": Noul("Is this ticket about billing?"),
			"sentiment": Choice("What is the customer's tone?", map[string]any{"calm": nil, "frustrated": nil, "angry": nil}),
			"urgency":   Score("How urgent is this ticket?", []any{"can wait", "this week", "today"}),
		}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Model == "" || result.Usage.InputTokens <= 0 || result.Usage.OutputTokens < 0 {
			t.Fatalf("metadata: %#v", result)
		}
		noul := result.Answers["isBilling"].(*NoulResponse)
		if noul.Noul < 0 || noul.Noul > 1 {
			t.Fatalf("noul: %#v", noul)
		}
		choice := result.Answers["sentiment"].(*ChoiceResponse)
		if !sameKeys(choice.Probabilities, []string{"angry", "calm", "frustrated"}) || !nearOne(sumProbabilities(choice.Probabilities)) {
			t.Fatalf("choice: %#v", choice)
		}
		score := result.Answers["urgency"].(*ScoreResponse)
		if score.Score < 0 || score.Score > 2 || !sameKeys(score.Probabilities, []string{"0", "1", "2"}) || !nearOne(sumProbabilities(score.Probabilities)) {
			t.Fatalf("score: %#v", score)
		}
	})

	t.Run("rich criteria", func(t *testing.T) {
		result, err := client.SystemOne(ctx, SystemOneRequest{State: ticket, Questions: Questions{
			"duplicate": Noul("Is the customer reporting a duplicate charge?", NoulCriteria{"true": map[string]any{"meaning": "the same amount charged more than once", "examples": []any{"billed twice"}}}),
			"tone":      Choice("Tone?", map[string]any{"calm": map[string]any{"summary": "measured", "examples": []any{"please look into this"}}, "upset": nil}),
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !sameKeys(result.Answers["tone"].(*ChoiceResponse).Probabilities, []string{"calm", "upset"}) {
			t.Fatal("wrong rich criteria labels")
		}
	})

	t.Run("bad key", func(t *testing.T) {
		bad, err := NewClient(WithAPIKey("not-a-real-key"), WithMaxRetries(0), WithLogLevel(LogOff))
		if err != nil {
			t.Fatal(err)
		}
		_, err = bad.Models.List(ctx)
		var authentication *AuthenticationError
		if !errors.As(err, &authentication) {
			t.Fatalf("got %T %v", err, err)
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		_, err := client.SystemOne(ctx, SystemOneRequest{State: "hello", Model: "no-such-model", Questions: Questions{"q": Noul("Is this a greeting?")}}, WithRequestMaxRetries(0))
		var bad *BadRequestError
		if !errors.As(err, &bad) || !strings.Contains(err.Error(), "Unknown model: no-such-model") || !strings.HasPrefix(bad.RequestID, "req_") {
			t.Fatalf("got %T %v", err, err)
		}
	})

	t.Run("server validation", func(t *testing.T) {
		_, err := client.SystemOne(ctx, SystemOneRequest{State: "x", Questions: Questions{"q": Score("?", []any{123, "ok"})}}, WithRequestMaxRetries(0))
		var validation *UnprocessableEntityError
		if !errors.As(err, &validation) || !strings.HasPrefix(err.Error(), "422 questions.q.score.criteria.0") {
			t.Fatalf("got %T %v", err, err)
		}
	})
}

func sumProbabilities(values map[string]float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum
}

func nearOne(value float64) bool { return value >= .9 && value <= 1.1 }

func sameKeys(values map[string]float64, want []string) bool {
	got := make([]string, 0, len(values))
	for key := range values {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
