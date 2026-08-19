package vectorscan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRoundTrip(t *testing.T) {
	db := NewDB()
	if err := db.Insert("a", []float32{1, 2, 3}, map[string]string{"kind": "alpha"}); err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	if err := db.Insert("b", []float32{4, 5, 6}, nil); err != nil {
		t.Fatalf("Insert b: %v", err)
	}

	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := db.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Dimension() != 3 {
		t.Fatalf("dimension: got %d want 3", loaded.Dimension())
	}
	if loaded.Len() != 2 {
		t.Fatalf("len: got %d want 2", loaded.Len())
	}

	got, ok := loaded.Get("a")
	if !ok {
		t.Fatal("expected vector a after reload")
	}
	if got.Values[0] != 1 || got.Values[1] != 2 || got.Values[2] != 3 {
		t.Fatalf("values did not survive reload: %v", got.Values)
	}
	if got.Metadata["kind"] != "alpha" {
		t.Fatalf("metadata did not survive reload: %v", got.Metadata)
	}
}

func TestLoadMissingSnapshot(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected missing snapshot error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadCorruptSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"dimension":3,"vectors":[`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected corrupt snapshot error")
	}
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("expected ErrCorruptSnapshot, got %v", err)
	}
}

func TestLoadRejectsDimensionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	data := []byte(`{"version":1,"dimension":3,"vectors":[{"id":"a","values":[1,2]}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("expected ErrCorruptSnapshot, got %v", err)
	}
}

func TestBuildSnapshotIsIndependent(t *testing.T) {
	db := NewDB()
	if err := db.Insert("a", []float32{1, 2, 3}, map[string]string{"kind": "original"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	s := db.buildSnapshot()
	s.Vectors[0].Values[0] = 999
	s.Vectors[0].Metadata["kind"] = "changed"

	got, ok := db.Get("a")
	if !ok {
		t.Fatal("expected vector")
	}
	if got.Values[0] != 1 || got.Metadata["kind"] != "original" {
		t.Fatalf("snapshot leaked DB-owned memory: %#v", got)
	}
}
