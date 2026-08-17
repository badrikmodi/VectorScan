package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	vectorscan "github.com/badrikmodi/VectorScan"
)

const maxBodyBytes = 4 << 20 // 4 MiB

type API struct {
	db  *vectorscan.DB
	mux *http.ServeMux
}

func New(db *vectorscan.DB) *API {
	a := &API{
		db:  db,
		mux: http.NewServeMux(),
	}

	a.mux.HandleFunc("POST /vectors", a.insertVector)
	a.mux.HandleFunc("GET /vectors/{id}", a.getVector)
	a.mux.HandleFunc("DELETE /vectors/{id}", a.deleteVector)
	a.mux.HandleFunc("POST /search", a.search)
	a.mux.HandleFunc("GET /healthz", a.health)
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

type insertRequest struct {
	ID       string            `json:"id"`
	Values   []float32         `json:"values"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (a *API) insertVector(w http.ResponseWriter, r *http.Request) {
	var req insertRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.db.Insert(req.ID, req.Values, req.Metadata); err != nil {
		writeDBError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     req.ID,
		"stored": true,
	})
}

func (a *API) getVector(w http.ResponseWriter, r *http.Request) {
	v, ok := a.db.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "vector not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) deleteVector(w http.ResponseWriter, r *http.Request) {
	if !a.db.Delete(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, "vector not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type searchRequest struct {
	Vector []float32 `json:"vector"`
	K      int       `json:"k"`
}

type searchResponse struct {
	Results []vectorscan.SearchResult `json:"results"`
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	results, err := a.db.Search(req.Vector, req.K)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results})
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"vectors":   a.db.Len(),
		"dimension": a.db.Dimension(),
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}

	var extra any
	if err := dec.Decode(&extra); err == nil {
		return errors.New("request body must contain exactly one JSON value")
	}
	return nil
}

func writeDBError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vectorscan.ErrEmptyID),
		errors.Is(err, vectorscan.ErrEmptyVector),
		errors.Is(err, vectorscan.ErrDimensionMismatch),
		errors.Is(err, vectorscan.ErrInvalidK):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
