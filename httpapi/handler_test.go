package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	vectorscan "github.com/badrikmodi/VectorScan"
)

func TestVectorLifecycleAndSearch(t *testing.T) {
	db := vectorscan.NewDB()
	api := New(db)

	insertBody := `{"id":"a","values":[1,0],"metadata":{"kind":"demo"}}`
	insertReq := httptest.NewRequest(http.MethodPost, "/vectors", strings.NewReader(insertBody))
	insertRec := httptest.NewRecorder()
	api.ServeHTTP(insertRec, insertReq)
	if insertRec.Code != http.StatusCreated {
		t.Fatalf("insert status: got %d body=%s", insertRec.Code, insertRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/vectors/a", nil)
	getRec := httptest.NewRecorder()
	api.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status: got %d body=%s", getRec.Code, getRec.Body.String())
	}

	var got vectorscan.Vector
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got.ID != "a" || len(got.Values) != 2 || got.Metadata["kind"] != "demo" {
		t.Fatalf("unexpected vector: %#v", got)
	}

	searchBody := `{"vector":[1,0],"k":1}`
	searchReq := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(searchBody))
	searchRec := httptest.NewRecorder()
	api.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status: got %d body=%s", searchRec.Code, searchRec.Body.String())
	}

	var searchOut struct {
		Results []vectorscan.SearchResult `json:"results"`
	}
	if err := json.NewDecoder(searchRec.Body).Decode(&searchOut); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(searchOut.Results) != 1 || searchOut.Results[0].ID != "a" {
		t.Fatalf("unexpected search response: %#v", searchOut)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/vectors/a", nil)
	deleteRec := httptest.NewRecorder()
	api.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status: got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/vectors/a", nil)
	missingRec := httptest.NewRecorder()
	api.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing status: got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestRejectDimensionMismatch(t *testing.T) {
	db := vectorscan.NewDB()
	api := New(db)

	_ = db.Insert("a", []float32{1, 2, 3}, nil)

	req := httptest.NewRequest(http.MethodPost, "/vectors", strings.NewReader(`{"id":"b","values":[1,2]}`))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
