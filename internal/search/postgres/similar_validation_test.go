package postgres

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryValidatesSimilarityQueryBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	tests := []struct {
		name        string
		provider    string
		model       string
		queryVector []float32
		source      *string
		limit       int
		contains    string
	}{
		{name: "provider", provider: " ", model: "model", queryVector: []float32{1}, limit: 1, contains: "provider is required"},
		{name: "model", provider: "provider", model: "\t", queryVector: []float32{1}, limit: 1, contains: "model is required"},
		{name: "vector", provider: "provider", model: "model", limit: 1, contains: "query vector is required"},
		{name: "zero norm", provider: "provider", model: "model", queryVector: []float32{0, 0}, limit: 1, contains: "query vector must have finite non-zero cosine norm"},
		{name: "NaN", provider: "provider", model: "model", queryVector: []float32{float32(math.NaN())}, limit: 1, contains: "value 0 must be finite"},
		{name: "blank source", provider: "provider", model: "model", queryVector: []float32{1}, source: similaritySource(" \t"), limit: 1, contains: "source is required when supplied"},
		{name: "zero limit", provider: "provider", model: "model", queryVector: []float32{1}, contains: "limit must be between"},
		{name: "negative limit", provider: "provider", model: "model", queryVector: []float32{1}, limit: -1, contains: "limit must be between"},
		{name: "high limit", provider: "provider", model: "model", queryVector: []float32{1}, limit: search.MaxSimilarSourceRecordsLimit + 1, contains: "limit must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := repository.SimilarSourceRecords(
				t.Context(), test.provider, test.model, test.queryVector, test.source, test.limit,
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("SimilarSourceRecords() error = %v, want containing %q", err, test.contains)
			}
			if got != nil {
				t.Errorf("SimilarSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func TestRepositoryPreservesSimilarityQueryFailures(t *testing.T) {
	queryErr := errors.New("query failed")
	scanErr := errors.New("scan failed")
	iterationErr := errors.New("iteration failed")
	tests := []struct {
		name     string
		db       failureDB
		contains string
	}{
		{name: "query", db: failureDB{queryErr: queryErr}, contains: "query similar source records"},
		{name: "cancellation", db: failureDB{queryErr: context.Canceled}, contains: "query similar source records"},
		{name: "scan", db: failureDB{rows: &failureRows{hasRow: true, scanErr: scanErr}}, contains: "scan similar source record"},
		{name: "iteration", db: failureDB{rows: &failureRows{rowsErr: iterationErr}}, contains: "iterate similar source records"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewRepository(test.db)
			if err != nil {
				t.Fatalf("NewRepository() error = %v", err)
			}
			got, err := repository.SimilarSourceRecords(t.Context(), " provider ", " model ", []float32{1}, nil, 1)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("SimilarSourceRecords() error = %v, want contextual %q", err, test.contains)
			}
			if !errors.Is(err, test.db.underlyingError()) {
				t.Errorf("SimilarSourceRecords() error = %v, want wrapping %v", err, test.db.underlyingError())
			}
			if got != nil {
				t.Errorf("SimilarSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func similaritySource(value string) *string {
	return &value
}

type failureDB struct {
	queryErr error
	rows     pgx.Rows
}

func (failureDB) Begin(context.Context) (pgx.Tx, error) {
	panic("similarity retrieval must not begin a transaction")
}

func (database failureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return database.rows, database.queryErr
}

func (database failureDB) underlyingError() error {
	if database.queryErr != nil {
		return database.queryErr
	}
	rows := database.rows.(*failureRows)
	if rows.scanErr != nil {
		return rows.scanErr
	}
	return rows.rowsErr
}

type failureRows struct {
	hasRow  bool
	scanErr error
	rowsErr error
}

func (rows *failureRows) Close()                                       {}
func (rows *failureRows) Err() error                                   { return rows.rowsErr }
func (rows *failureRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (rows *failureRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (rows *failureRows) Next() bool {
	if !rows.hasRow {
		return false
	}
	rows.hasRow = false
	return true
}
func (rows *failureRows) Scan(...any) error      { return rows.scanErr }
func (rows *failureRows) Values() ([]any, error) { return nil, nil }
func (rows *failureRows) RawValues() [][]byte    { return nil }
func (rows *failureRows) Conn() *pgx.Conn        { return nil }
