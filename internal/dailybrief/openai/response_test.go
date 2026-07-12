package openai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/dailybrief"
)

func TestOpenAIDailyBriefGeneratorRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name, body, contains string
	}{
		{name: "malformed envelope", body: `{`, contains: "decode response envelope"},
		{name: "incomplete", body: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, contains: "response is incomplete: max_output_tokens"},
		{name: "failed", body: `{"status":"failed","error":{"message":"internal details","type":"server_error","code":"failed"}}`, contains: "response failed"},
		{name: "completed with error", body: `{"status":"completed","error":{"message":"internal details","type":"server_error","code":"failed"}}`, contains: "completed response contains a provider error"},
		{name: "missing message", body: `{"status":"completed","output":[]}`, contains: "0 assistant messages"},
		{name: "duplicate message", body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}"}]}]}`, contains: "2 assistant messages"},
		{name: "unexpected output", body: `{"status":"completed","output":[{"type":"function_call"}]}`, contains: `unexpected output item type "function_call"`},
		{name: "missing content", body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[]}]}`, contains: "0 content items, want 1"},
		{name: "unexpected content", body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"image"}]}]}`, contains: `unexpected message content type "image"`},
		{name: "refusal", body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}]}`, contains: "refused to generate"},
		{name: "malformed structured output", body: completedOpenAIResponse(`{`), contains: "decode structured daily brief"},
		{name: "unknown structured field", body: completedOpenAIResponse(`{"sections": [], "unexpected": true}`), contains: `unknown field "unexpected"`},
		{name: "trailing structured JSON", body: completedOpenAIResponse(`{"sections": []} {}`), contains: "unexpected trailing JSON value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				writeOpenAIResponse(t, response, http.StatusOK, test.body)
			}))
			defer server.Close()
			got, err := newOpenAITestGenerator(t, server).Generate(t.Context(), generationInput())
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Generate() error = %v, want error containing %q", err, test.contains)
			}
			if !reflect.DeepEqual(got, dailybrief.Generation{}) {
				t.Errorf("Generate() = %#v, want zero value", got)
			}
		})
	}
}
