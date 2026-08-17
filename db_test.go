package vectorscan

import (
	"math"
	"sync"
	"testing"
)

func TestInsertCopiesInput(t *testing.T) {
	db := NewDB()
	values := []float32{1, 2, 3}
	metadata := map[string]string{"kind": "test"}

	if err := db.Insert("a", values, metadata); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	values[0] = 999
	metadata["kind"] = "changed"

	got, ok := db.Get("a")
	if !ok {
		t.Fatal("expected vector")
	}
	if got.Values[0] != 1 {
		t.Fatalf("stored vector mutated through caller slice: got %v", got.Values)
	}
	if got.Metadata["kind"] != "test" {
		t.Fatalf("stored metadata mutated through caller map: got %v", got.Metadata)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	db := NewDB()
	if err := db.Insert("a", []float32{1, 2}, map[string]string{"x": "y"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	first, _ := db.Get("a")
	first.Values[0] = 42
	first.Metadata["x"] = "changed"

	second, _ := db.Get("a")
	if second.Values[0] != 1 || second.Metadata["x"] != "y" {
		t.Fatalf("Get leaked DB-owned memory: %#v", second)
	}
}

func TestDelete(t *testing.T) {
	db := NewDB()
	_ = db.Insert("a", []float32{1}, nil)

	if !db.Delete("a") {
		t.Fatal("expected existing vector to be deleted")
	}
	if db.Delete("a") {
		t.Fatal("second delete should report missing vector")
	}
	if _, ok := db.Get("a"); ok {
		t.Fatal("deleted vector still exists")
	}
}

func TestDimensionMismatch(t *testing.T) {
	db := NewDB()
	if err := db.Insert("a", []float32{1, 2, 3}, nil); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := db.Insert("b", []float32{1, 2}, nil); err != ErrDimensionMismatch {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}
}

func TestSearchCosineTopK(t *testing.T) {
	db := NewDB()
	_ = db.Insert("x", []float32{1, 0}, nil)
	_ = db.Insert("y", []float32{0, 1}, nil)
	_ = db.Insert("diag", []float32{1, 1}, nil)

	results, err := db.Search([]float32{1, 0}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "x" {
		t.Fatalf("expected x first, got %#v", results)
	}
	if math.Abs(float64(results[0].Score-1)) > 1e-6 {
		t.Fatalf("expected cosine score 1, got %f", results[0].Score)
	}
	if results[1].ID != "diag" {
		t.Fatalf("expected diag second, got %#v", results)
	}
}

func TestConcurrentReadersAndWriters(t *testing.T) {
	db := NewDB()
	const workers = 20

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = db.Insert("shared", []float32{1, 2, 3}, nil)
			_, _ = db.Get("shared")
			_, _ = db.Search([]float32{1, 2, 3}, 1)
		}( )
	}
	wg.Wait()
}
