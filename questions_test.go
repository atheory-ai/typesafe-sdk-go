package typesafe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQuestionBuildersMarshalWireFormat(t *testing.T) {
	questions := Questions{
		"noul":   Noul(nil, NoulCriteria{"true": nil, "false": "no"}),
		"choice": Choice("which?", map[string]any{"a": nil, "b": map[string]any{"why": "yes"}}),
		"score":  Score([]any{"instructions"}, []any{"low", nil, "high"}),
	}
	got, err := json.Marshal(questions)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"choice":{"criteria":{"a":null,"b":{"why":"yes"}},"instructions":"which?","type":"choice"},"noul":{"criteria":{"false":"no","true":null},"instructions":null,"type":"noul"},"score":{"criteria":["low",null,"high"],"instructions":["instructions"],"type":"score"}}`
	if string(got) != want {
		t.Fatalf("wire format mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestValidateQuestions(t *testing.T) {
	tests := []struct {
		name string
		qs   Questions
		want string
	}{
		{"empty", Questions{}, "At least one question is required."},
		{"score empty", Questions{"q": Score("q", nil)}, `Score question "q" has 0 criteria; at least two scores are required.`},
		{"score one", Questions{"q": Score("q", []any{"only"})}, `Score question "q" has 1 criteria; at least two scores are required.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuestions(tt.qs)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodeAnswers(t *testing.T) {
	var result SystemOneResult
	err := json.Unmarshal([]byte(`{
		"model":"jev-1","answers":{
			"yes":{"type":"noul","noul":0.75},
			"pick":{"type":"choice","choice":"b","confidence":0.9,"probabilities":{"a":0.1,"b":0.9}},
			"rate":{"type":"score","score":1.4,"confidence":0.8,"legend":{"0":"bad","1":"ok","2":"good"},"probabilities":{"0":0.1,"1":0.4,"2":0.5}}
		},"usage":{"input_tokens":2,"output_tokens":3}}`), &result)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := result.Answers["yes"].(*NoulResponse); !ok || got.Noul != .75 {
		t.Fatalf("unexpected noul answer: %#v", result.Answers["yes"])
	}
	if got, ok := result.Answers["pick"].(*ChoiceResponse); !ok || got.Choice != "b" {
		t.Fatalf("unexpected choice answer: %#v", result.Answers["pick"])
	}
	if got, ok := result.Answers["rate"].(*ScoreResponse); !ok || got.Score != 1.4 {
		t.Fatalf("unexpected score answer: %#v", result.Answers["rate"])
	}
	if result.Answers["yes"].AnswerType() != "noul" || result.Answers["pick"].AnswerType() != "choice" || result.Answers["rate"].AnswerType() != "score" {
		t.Fatal("answer type methods disagree with wire types")
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"answers":{"pick"`) {
		t.Fatalf("result did not marshal answers: %s, %v", encoded, err)
	}
}

func TestQuestionTypesAndOmittedFields(t *testing.T) {
	questions := Questions{
		"n": NoulQuestion{},
		"c": ChoiceQuestion{Criteria: map[string]any{"a": nil}},
		"s": ScoreQuestion{Criteria: []any{"low", "high"}},
	}
	if questions["n"].QuestionType() != "noul" || questions["c"].QuestionType() != "choice" || questions["s"].QuestionType() != "score" {
		t.Fatal("wrong question type")
	}
	data, err := json.Marshal(questions)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"c":{"criteria":{"a":null},"type":"choice"},"n":{"type":"noul"},"s":{"criteria":["low","high"],"type":"score"}}` {
		t.Fatalf("omitted fields changed: %s", data)
	}
}

func TestDecodeRejectsUnknownAndMalformedAnswers(t *testing.T) {
	for _, body := range []string{
		`{"answers":{"q":{"type":"future"}}}`,
		`{"answers":{"q":[]}}`,
		`{"answers":{"q":{"type":"noul","noul":"bad"}}}`,
	} {
		var result SystemOneResult
		if err := json.Unmarshal([]byte(body), &result); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestNilQuestionRejected(t *testing.T) {
	if err := ValidateQuestions(Questions{"q": nil}); err == nil {
		t.Fatal("accepted nil question")
	}
}
