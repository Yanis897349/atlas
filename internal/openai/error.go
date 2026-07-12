package openai

import (
	"encoding/json"
	"fmt"
	"strings"
)

type errorResponse struct {
	Error *errorDetails `json:"error"`
}

type errorDetails struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ProviderError returns a sanitized error for one unsuccessful OpenAI API response.
func ProviderError(apiName string, statusCode int, body []byte) error {
	var response errorResponse
	if err := json.Unmarshal(body, &response); err != nil || response.Error == nil {
		return fmt.Errorf("%s returned status %d", apiName, statusCode)
	}

	parts := make([]string, 0, 3)
	for _, value := range []string{response.Error.Type, response.Error.Code, response.Error.Message} {
		if value = SanitizeErrorValue(value); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("%s returned status %d", apiName, statusCode)
	}
	return fmt.Errorf("%s returned status %d: %s", apiName, statusCode, strings.Join(parts, ": "))
}

// SanitizeErrorValue bounds and flattens provider-controlled error text.
func SanitizeErrorValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxLength = 256
	if len(value) > maxLength {
		return value[:maxLength] + "..."
	}
	return value
}
