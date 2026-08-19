package vectorscan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const snapshotVersion = 1

var ErrCorruptSnapshot = errors.New("corrupt snapshot")

// snapshot is the durable representation of the database.
// It is intentionally separate from DB so the in-memory layout can evolve
// without coupling it to the on-disk format.
type snapshot struct {
	Version   int              `json:"version"`
	Dimension int              `json:"dimension"`
	Vectors   []snapshotVector `json:"vectors"`
}

type snapshotVector struct {
	ID       string            `json:"id"`
	Values   []float32         `json:"values"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// buildSnapshot creates an independent point-in-time copy of durable DB state.
// The read lock is held only while copying RAM. Encoding and disk I/O happen
// after the lock has been released so slow storage does not extend the write pause.
func (db *DB) buildSnapshot() snapshot {
	db.mu.RLock()
	defer db.mu.RUnlock()

	s := snapshot{
		Version:   snapshotVersion,
		Dimension: db.dimension,
		Vectors:   make([]snapshotVector, 0, len(db.vectors)),
	}

	for _, v := range db.vectors {
		s.Vectors = append(s.Vectors, snapshotVector{
			ID:       v.ID,
			Values:   cloneValues(v.Values),
			Metadata: cloneMetadata(v.Metadata),
		})
	}
	return s
}

// Save writes a crash-resistant point-in-time snapshot.
// It writes and fsyncs a temporary file first, then atomically renames it over
// the destination and fsyncs the parent directory so the rename is durable.
func (db *DB) Save(path string) error {
	if path == "" {
		return errors.New("snapshot path cannot be empty")
	}

	s := db.buildSnapshot()
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+"-tmp-*")
	if err != nil {
		return fmt.Errorf("create snapshot temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	enc := json.NewEncoder(tmp)
	if err := enc.Encode(&s); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync snapshot temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close snapshot temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	cleanup = false

	// Persist the directory entry update where the platform supports directory sync.
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open snapshot directory: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync snapshot directory: %w", err)
	}
	return nil
}

// Load reads, validates, and reconstructs a DB from a snapshot file.
// Missing files preserve os.ErrNotExist so callers can distinguish first boot
// from a corrupt snapshot. Format/validation failures wrap ErrCorruptSnapshot.
func Load(path string) (*DB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()

	var s snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrCorruptSnapshot, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: multiple JSON values", ErrCorruptSnapshot)
		}
		return nil, fmt.Errorf("%w: trailing data: %v", ErrCorruptSnapshot, err)
	}

	if err := validateSnapshot(s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptSnapshot, err)
	}

	db := NewDB()
	db.dimension = s.Dimension
	for _, v := range s.Vectors {
		db.vectors[v.ID] = &Vector{
			ID:       v.ID,
			Values:   cloneValues(v.Values),
			Metadata: cloneMetadata(v.Metadata),
		}
	}
	return db, nil
}

func validateSnapshot(s snapshot) error {
	if s.Version != snapshotVersion {
		return fmt.Errorf("unsupported snapshot version %d", s.Version)
	}
	if s.Dimension < 0 {
		return errors.New("dimension cannot be negative")
	}
	if len(s.Vectors) > 0 && s.Dimension == 0 {
		return errors.New("non-empty snapshot has zero dimension")
	}

	seen := make(map[string]struct{}, len(s.Vectors))
	for _, v := range s.Vectors {
		if v.ID == "" {
			return ErrEmptyID
		}
		if len(v.Values) == 0 {
			return fmt.Errorf("vector %q: %w", v.ID, ErrEmptyVector)
		}
		if len(v.Values) != s.Dimension {
			return fmt.Errorf("vector %q: %w", v.ID, ErrDimensionMismatch)
		}
		if _, ok := seen[v.ID]; ok {
			return fmt.Errorf("duplicate vector id %q", v.ID)
		}
		seen[v.ID] = struct{}{}
	}
	return nil
}
