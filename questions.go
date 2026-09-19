package typesafe

// Noul creates a yes/no question. A nil criteria map omits criteria; entries
// present with nil values are sent as JSON null.
func Noul(instructions Entry, criteria ...NoulCriteria) NoulQuestion {
	q := NoulQuestion{Type: "noul", Instructions: instructions, HasInstructions: true}
	if len(criteria) > 0 {
		q.Criteria = criteria[0]
		q.HasCriteria = true
	}
	return q
}

// Choice creates a question that selects one named alternative.
func Choice(instructions Entry, criteria map[string]Entry) ChoiceQuestion {
	return ChoiceQuestion{Type: "choice", Instructions: instructions, Criteria: criteria, HasInstructions: true}
}

// Score creates a question using an ordered rubric indexed from zero.
func Score(instructions Entry, criteria []Entry) ScoreQuestion {
	return ScoreQuestion{Type: "score", Instructions: instructions, Criteria: criteria, HasInstructions: true}
}

// ValidateQuestions rejects empty question sets and invalid score rubrics.
func ValidateQuestions(questions Questions) error {
	if len(questions) == 0 {
		return &TypeSafeError{Message: "At least one question is required."}
	}
	for name, question := range questions {
		if question == nil {
			return &TypeSafeError{Message: "Question " + name + " is nil."}
		}
		if err := question.validate(name); err != nil {
			return err
		}
	}
	return nil
}
