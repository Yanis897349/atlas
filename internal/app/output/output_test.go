package output

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEncodeJSONBufferedWritesExactUnescapedJSON(t *testing.T) {
	var stdout bytes.Buffer
	if err := EncodeJSONBuffered(&stdout, "test result", map[string]string{"value": "<tag>&"}); err != nil {
		t.Fatalf("EncodeJSONBuffered() error = %v", err)
	}
	if got, want := stdout.String(), "{\"value\":\"<tag>&\"}\n"; got != want {
		t.Errorf("EncodeJSONBuffered() output = %q, want %q", got, want)
	}
}

func TestEncodeJSONBufferedDoesNotWriteEncodingFailures(t *testing.T) {
	var stdout bytes.Buffer
	err := EncodeJSONBuffered(&stdout, "test result", map[string]any{"unsupported": make(chan struct{})})
	if err == nil || !strings.Contains(err.Error(), "encode test result") || stdout.Len() != 0 {
		t.Fatalf("EncodeJSONBuffered() = (%v, %q), want contextual error without output", err, stdout.String())
	}
}

func TestEncodeJSONBufferedPreservesWriterFailures(t *testing.T) {
	wantErr := errors.New("writer unavailable")
	err := EncodeJSONBuffered(errorWriter{err: wantErr}, "test result", []string{})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "write test result") {
		t.Fatalf("EncodeJSONBuffered() error = %v, want contextual writer failure", err)
	}
}

func TestEncodeJSONBufferedRejectsShortWrites(t *testing.T) {
	err := EncodeJSONBuffered(shortWriter{}, "test result", []string{})
	if !errors.Is(err, io.ErrShortWrite) || !strings.Contains(err.Error(), "write test result") {
		t.Fatalf("EncodeJSONBuffered() error = %v, want contextual io.ErrShortWrite", err)
	}
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}
