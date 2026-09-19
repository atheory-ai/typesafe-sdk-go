package typesafe

import (
	"encoding/json"
	"fmt"
)

// Entry is a JSON-compatible value used for state, instructions, and criteria.
type Entry = any

// Question is one of the question primitives accepted by System One.
type Question interface {
	QuestionType() string
	validate(name string) error
}

// Questions identifies questions by the names used for their answers.
type Questions map[string]Question

// NoulCriteria optionally describes the true and false outcomes.
// Only the keys "true" and "false" have meaning to the API.
type NoulCriteria map[string]Entry

type NoulQuestion struct {
	Type            string       `json:"-"`
	Instructions    Entry        `json:"-"`
	Criteria        NoulCriteria `json:"-"`
	HasInstructions bool         `json:"-"`
	HasCriteria     bool         `json:"-"`
}

func (q NoulQuestion) QuestionType() string  { return "noul" }
func (q NoulQuestion) validate(string) error { return nil }
func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	wire := map[string]any{"type": "noul"}
	if q.HasInstructions {
		wire["instructions"] = q.Instructions
	}
	if q.HasCriteria {
		wire["criteria"] = q.Criteria
	}
	return json.Marshal(wire)
}

type ChoiceQuestion struct {
	Type            string           `json:"-"`
	Instructions    Entry            `json:"-"`
	Criteria        map[string]Entry `json:"-"`
	HasInstructions bool             `json:"-"`
}

func (q ChoiceQuestion) QuestionType() string  { return "choice" }
func (q ChoiceQuestion) validate(string) error { return nil }
func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	wire := map[string]any{"type": "choice", "criteria": q.Criteria}
	if q.HasInstructions {
		wire["instructions"] = q.Instructions
	}
	return json.Marshal(wire)
}

type ScoreQuestion struct {
	Type            string  `json:"-"`
	Instructions    Entry   `json:"-"`
	Criteria        []Entry `json:"-"`
	HasInstructions bool    `json:"-"`
}

func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	wire := map[string]any{"type": "score", "criteria": q.Criteria}
	if q.HasInstructions {
		wire["instructions"] = q.Instructions
	}
	return json.Marshal(wire)
}

func (q ScoreQuestion) QuestionType() string { return "score" }
func (q ScoreQuestion) validate(name string) error {
	if len(q.Criteria) < 2 {
		return &TypeSafeError{Message: fmt.Sprintf(
			"Score question %q has %d criteria; at least two scores are required.", name, len(q.Criteria))}
	}
	return nil
}

// Answer is implemented by all System One answer primitives.
type Answer interface{ AnswerType() string }

type NoulResponse struct {
	Type string  `json:"type"`
	Noul float64 `json:"noul"`
}

func (a NoulResponse) AnswerType() string { return "noul" }

type ChoiceResponse struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

func (a ChoiceResponse) AnswerType() string { return "choice" }

type ScoreResponse struct {
	Type          string             `json:"type"`
	Score         float64            `json:"score"`
	Confidence    float64            `json:"confidence"`
	Legend        map[string]Entry   `json:"legend"`
	Probabilities map[string]float64 `json:"probabilities"`
}

func (a ScoreResponse) AnswerType() string { return "score" }

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type SystemOneResult struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"-"`
	Usage   Usage             `json:"usage"`
}

func (r SystemOneResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Model   string            `json:"model"`
		Answers map[string]Answer `json:"answers"`
		Usage   Usage             `json:"usage"`
	}{Model: r.Model, Answers: r.Answers, Usage: r.Usage})
}

func (r *SystemOneResult) UnmarshalJSON(data []byte) error {
	var wire struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
		Usage   Usage                      `json:"usage"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	answers := make(map[string]Answer, len(wire.Answers))
	for name, raw := range wire.Answers {
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &kind); err != nil {
			return fmt.Errorf("decode answer %q: %w", name, err)
		}
		var answer Answer
		switch kind.Type {
		case "noul":
			answer = &NoulResponse{}
		case "choice":
			answer = &ChoiceResponse{}
		case "score":
			answer = &ScoreResponse{}
		default:
			return fmt.Errorf("decode answer %q: unknown answer type %q", name, kind.Type)
		}
		if err := json.Unmarshal(raw, answer); err != nil {
			return fmt.Errorf("decode answer %q: %w", name, err)
		}
		answers[name] = answer
	}
	r.Model, r.Answers, r.Usage = wire.Model, answers, wire.Usage
	return nil
}

type ModelCard struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}

// SystemOneRequest is the body sent to POST /v1/systemone. Extra fields are
// forwarded verbatim; explicit fields take precedence over conflicting extras.
type SystemOneRequest struct {
	State     Entry
	Questions Questions
	Model     string
	Extra     map[string]Entry
}

func (r SystemOneRequest) payload(defaultModel string) map[string]any {
	body := make(map[string]any, len(r.Extra)+3)
	for key, value := range r.Extra {
		body[key] = value
	}
	body["state"] = r.State
	body["questions"] = r.Questions
	if r.Model == "" {
		body["model"] = defaultModel
	} else {
		body["model"] = r.Model
	}
	return body
}
