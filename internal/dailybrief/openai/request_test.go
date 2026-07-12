package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/dailybrief"
)

func TestOpenAIDailyBriefGeneratorBoundsRequestConstruction(t *testing.T) {
	tests := []struct {
		name     string
		ctx      func() context.Context
		input    func() dailybrief.Input
		contains string
	}{
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			input: generationInput, contains: "context canceled",
		},
		{
			name: "oversized input", ctx: t.Context,
			input: func() dailybrief.Input {
				input := generationInput()
				input.SourceRecords[0].Title = strings.Repeat("x", maxOpenAIDailyBriefInputBytes+1)
				return input
			},
			contains: "daily brief input is too large",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &openAIHTTPClientStub{}
			generator, err := NewGenerator(Config{APIKey: "key", Model: "model", Client: client})
			if err != nil {
				t.Fatalf("newOpenAIDailyBriefGenerator() error = %v", err)
			}
			_, err = generator.Generate(test.ctx(), test.input())
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Generate() error = %v, want error containing %q", err, test.contains)
			}
			if client.calls != 0 {
				t.Errorf("HTTP calls = %d, want 0", client.calls)
			}
		})
	}
}
