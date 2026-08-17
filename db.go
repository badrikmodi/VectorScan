package vectorscan

import (
	"errors"
	"math"
	"sort"
	"sync"
)

var (
	ErrEmptyID           = errors.New("id cannot be empty")
	ErrEmptyVector       = errors.New("vector cannot be empty")
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrInvalidK          = errors.New("k must be greater than zero")
)

// Vector is the record stored by the database.
// Values and Metadata are owned by DB after insertion.
type Vector struct {
	ID       string            `json:"id"`
	Values   []float32         `json:"values"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SearchResult is one nearest-neighbor result.
type SearchResult struct {
	ID    string  `json:"id"`
	Score float32 `json:"score"`
}

// DB is the V0-V3 in-memory storage engine.
//
// One DB instance is created at process startup and shared by all HTTP handlers.
// mu protects vectors and dimension.
type DB struct {
	mu        sync.RWMutex
	vectors   map[string]*Vector
	dimension int
}

func NewDB() *DB {
	return &DB{
		vectors: make(map[string]*Vector),
	}
}

// Insert stores or replaces a vector.
// The input slice and metadata map are copied so callers cannot mutate DB-owned memory.
func (db *DB) Insert(id string, values []float32, metadata map[string]string) error {
	if id == "" {
		return ErrEmptyID
	}
	if len(values) == 0 {
		return ErrEmptyVector
	}

	valuesCopy := cloneValues(values)
	metadataCopy := cloneMetadata(metadata)

	db.mu.Lock()
	defer db.mu.Unlock()

	if db.dimension == 0 {
		db.dimension = len(values)
	} else if len(values) != db.dimension {
		return ErrDimensionMismatch
	}

	db.vectors[id] = &Vector{
		ID:       id,
		Values:   valuesCopy,
		Metadata: metadataCopy,
	}
	return nil
}

// Get returns a copy of the stored vector so callers cannot mutate DB-owned memory.
func (db *DB) Get(id string) (*Vector, bool) {
	db.mu.RLock()
	v, ok := db.vectors[id]
	if !ok {
		db.mu.RUnlock()
		return nil, false
	}

	out := &Vector{
		ID:       v.ID,
		Values:   cloneValues(v.Values),
		Metadata: cloneMetadata(v.Metadata),
	}
	db.mu.RUnlock()
	return out, true
}

// Delete removes an ID and reports whether it existed.
func (db *DB) Delete(id string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, ok := db.vectors[id]; !ok {
		return false
	}
	delete(db.vectors, id)
	return true
}

// Search performs an exact brute-force cosine-similarity scan.
// V1 deliberately sorts all N scores, making this O(N*d + N log N).
// A bounded heap can replace the full sort later without changing the API.
func (db *DB) Search(query []float32, k int) ([]SearchResult, error) {
	if k <= 0 {
		return nil, ErrInvalidK
	}
	if len(query) == 0 {
		return nil, ErrEmptyVector
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.dimension != 0 && len(query) != db.dimension {
		return nil, ErrDimensionMismatch
	}

	results := make([]SearchResult, 0, len(db.vectors))
	for id, v := range db.vectors {
		results = append(results, SearchResult{
			ID:    id,
			Score: cosineSimilarity(query, v.Values),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if k < len(results) {
		results = results[:k]
	}
	return results, nil
}

// Len returns the current number of stored vectors.
func (db *DB) Len() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.vectors)
}

// Dimension returns the fixed vector dimension, or 0 before the first insert.
func (db *DB) Dimension() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.dimension
}

func cosineSimilarity(a, b []float32) float32 {
	var dot float64
	var normA float64
	var normB float64

	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

func cloneValues(values []float32) []float32 {
	if values == nil {
		return nil
	}
	out := make([]float32, len(values))
	copy(out, values)
	return out
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for k, v := range metadata {
		out[k] = v
	}
	return out
}
