package typesafe_test

import (
	"context"
	"log"

	typesafe "github.com/atheory-ai/typesafe-sdk-go"
)

func ExampleClient_SystemOne() {
	client, err := typesafe.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.SystemOne(context.Background(), typesafe.SystemOneRequest{
		State: "I was charged twice and need a refund.",
		Questions: typesafe.Questions{
			"category": typesafe.Choice("What is this about?", map[string]any{
				"billing": nil, "technical": nil, "other": nil,
			}),
			"urgent": typesafe.Noul("Does this require immediate attention?"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	category := result.Answers["category"].(*typesafe.ChoiceResponse)
	_ = category.Choice
}
