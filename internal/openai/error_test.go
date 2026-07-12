package openai

import (
	"fmt"
	"strings"
	"testing"
)

func TestProviderErrorSanitizesStructuredErrors(t *testing.T) {
	message := "quota\nexceeded " + strings.Repeat("private ", 100)
	err := ProviderError("OpenAI Test API", 429, []byte(fmt.Sprintf(`{
		"error":{"message":%q,"type":"rate_limit_error","code":"rate_limit_exceeded"}
	}`, message)))
	if !strings.Contains(err.Error(), "OpenAI Test API returned status 429: rate_limit_error: rate_limit_exceeded: quota exceeded") {
		t.Fatalf("ProviderError() = %v", err)
	}
	if strings.Contains(err.Error(), "\n") || len(err.Error()) > 400 {
		t.Errorf("ProviderError() exposed unsanitized provider data: %q", err)
	}
}

func TestProviderErrorFallsBackForMalformedOrEmptyDetails(t *testing.T) {
	for _, body := range [][]byte{[]byte("not JSON"), []byte(`{"error":null}`), []byte(`{"error":{}}`)} {
		err := ProviderError("OpenAI Test API", 500, body)
		if err.Error() != "OpenAI Test API returned status 500" {
			t.Errorf("ProviderError(%q) = %q", body, err)
		}
	}
}
