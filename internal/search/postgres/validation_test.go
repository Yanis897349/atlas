package postgres

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryValidatesEmbeddingBatchBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	valid := []search.SourceRecordEmbedding{
		{
			SourceRecordID: "abcdefab-cdef-abcd-efab-cdefabcdefab",
			Provider:       "provider",
			Model:          "model",
			Vector:         []float32{0.1, 0.2},
		},
		{
			SourceRecordID: "00000000-0000-0000-0000-000000000002",
			Provider:       "provider",
			Model:          "model",
			Vector:         []float32{0.3, 0.4},
		},
	}
	tests := []struct {
		name       string
		embeddings []search.SourceRecordEmbedding
		actor      string
		contains   string
	}{
		{name: "actor", embeddings: valid, actor: " ", contains: "actor is required"},
		{name: "source record ID", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.SourceRecordID = "bad" }), actor: "actor", contains: "source record ID must be a UUID"},
		{name: "provider", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Provider = "\t" }), actor: "actor", contains: "provider is required"},
		{name: "model", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Model = " " }), actor: "actor", contains: "model is required"},
		{name: "vector", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Vector = nil }), actor: "actor", contains: "vector is required"},
		{name: "zero norm", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Vector = []float32{0, 0} }), actor: "actor", contains: "vector must have finite non-zero cosine norm"},
		{name: "underflowing norm", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Vector = []float32{1e-30} }), actor: "actor", contains: "vector must have finite non-zero cosine norm"},
		{name: "overflowing norm", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Vector = []float32{1e30} }), actor: "actor", contains: "vector must have finite non-zero cosine norm"},
		{name: "dimension", embeddings: updateEmbedding(valid, 1, func(value *search.SourceRecordEmbedding) { value.Vector = []float32{0.3} }), actor: "actor", contains: "does not match batch dimension"},
		{name: "NaN", embeddings: updateEmbedding(valid, 0, func(value *search.SourceRecordEmbedding) { value.Vector[0] = float32(math.NaN()) }), actor: "actor", contains: "must be finite"},
		{name: "infinity", embeddings: updateEmbedding(valid, 1, func(value *search.SourceRecordEmbedding) { value.Vector[1] = float32(math.Inf(1)) }), actor: "actor", contains: "must be finite"},
		{name: "duplicate provenance", embeddings: updateEmbedding(valid, 1, func(value *search.SourceRecordEmbedding) { value.SourceRecordID = valid[0].SourceRecordID }), actor: "actor", contains: "duplicates source record"},
		{name: "duplicate UUID alias", embeddings: updateEmbedding(valid, 1, func(value *search.SourceRecordEmbedding) {
			value.SourceRecordID = strings.ToUpper(strings.ReplaceAll(valid[0].SourceRecordID, "-", ""))
		}), actor: "actor", contains: "duplicates source record"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := repository.PersistSourceRecordEmbeddings(t.Context(), test.embeddings, test.actor)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("PersistSourceRecordEmbeddings() error = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestRepositoryAcceptsEmptyEmbeddingBatchWithoutPostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if err := repository.PersistSourceRecordEmbeddings(t.Context(), nil, " actor "); err != nil {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v", err)
	}
}

func TestNewRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
	}
}

func updateEmbedding(
	embeddings []search.SourceRecordEmbedding,
	index int,
	update func(*search.SourceRecordEmbedding),
) []search.SourceRecordEmbedding {
	result := append([]search.SourceRecordEmbedding(nil), embeddings...)
	for resultIndex := range result {
		result[resultIndex].Vector = append([]float32(nil), result[resultIndex].Vector...)
	}
	update(&result[index])
	return result
}

type panicDB struct{}

func (panicDB) Begin(context.Context) (pgx.Tx, error) {
	panic("validation must happen before beginning a transaction")
}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}
