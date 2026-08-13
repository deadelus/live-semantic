package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newCollectionTermParamContext is newParamContext (gallery_test.go) with
// two URL params — needed for /collections/:name/terms/:term. Not named
// newTwoParamContext (already taken by gallery_test.go's own two-param
// helper, fixed to "name"/"imageID" keys — this one needs "name"/"term").
func newCollectionTermParamContext(method, path, body, p1Name, p1Value, p2Name, p2Value string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reqBody *bytes.Buffer
	if body == "" {
		reqBody = bytes.NewBuffer(nil)
	} else {
		reqBody = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: p1Name, Value: p1Value}, {Key: p2Name, Value: p2Value}}
	return c, w
}

func TestCollectionsAdd_ValidRequest_ReturnsCreated(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, w := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"Manchester Team","tags":["football","2026"]}`, "", "")
	cc.add(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if _, ok := mock.collections["Manchester Team"]; !ok {
		t.Fatal("collection not found after add()")
	}
}

func TestCollectionsAdd_DuplicateName_ReturnsBadRequest(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)

	c, w := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (duplicate name)", w.Code, http.StatusBadRequest)
	}
}

func TestCollectionsList_ReturnsCollections(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"a"}`, "", "")
	cc.add(c)
	c, _ = newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"b"}`, "", "")
	cc.add(c)

	c, w := newParamContext(http.MethodGet, "/api/v1/collections", "", "", "")
	cc.list(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(mock.collections) != 2 {
		t.Fatalf("collections = %+v, want 2 entries", mock.collections)
	}
}

func TestCollectionsRemove_IsIdempotent(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)

	c, w := newParamContext(http.MethodDelete, "/api/v1/collections/team", "", "name", "team")
	cc.remove(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if _, ok := mock.collections["team"]; ok {
		t.Fatal("collection still present after remove()")
	}

	// Idempotent — removing again must still be 200, not an error.
	c, w = newParamContext(http.MethodDelete, "/api/v1/collections/team", "", "name", "team")
	cc.remove(c)
	if w.Code != http.StatusOK {
		t.Fatalf("second remove() status = %d, want %d (idempotent)", w.Code, http.StatusOK)
	}
}

func TestCollectionsUpdate_RenameAndTags(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"old","tags":["a"]}`, "", "")
	cc.add(c)

	c, w := newParamContext(http.MethodPatch, "/api/v1/collections/old", `{"new_name":"new","tags":["b","c"]}`, "name", "old")
	cc.update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	entry, ok := mock.collections["new"]
	if !ok {
		t.Fatal("renamed collection not found")
	}
	if len(entry.Tags) != 2 {
		t.Fatalf("Tags = %v, want 2 tags", entry.Tags)
	}
	if _, stillOld := mock.collections["old"]; stillOld {
		t.Fatal("old name should no longer resolve after rename")
	}
}

func TestCollectionsAddTerm_ForwardsCollectionAndTermNames(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"thing": true}}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)

	c, w := newCollectionTermParamContext(http.MethodPost, "/api/v1/collections/team/terms/thing", "", "name", "team", "term", "thing")
	cc.addTerm(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := mock.collections["team"].Terms; len(got) != 1 || got[0] != "thing" {
		t.Fatalf("Terms = %v, want [thing]", got)
	}
}

// TestCollectionsAddTerm_UnknownTermRejected — see
// uc.AddTermToCollection's doc comment: a Collection referencing a name
// that isn't a real Term yet must be rejected, not silently accepted.
func TestCollectionsAddTerm_UnknownTermRejected(t *testing.T) {
	mock := &mockUseCases{}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)

	c, w := newCollectionTermParamContext(http.MethodPost, "/api/v1/collections/team/terms/does-not-exist", "", "name", "team", "term", "does-not-exist")
	cc.addTerm(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (unknown term)", w.Code, http.StatusBadRequest)
	}
}

func TestCollectionsRemoveTerm_IsIdempotent(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"thing": true}}
	cc := newCollectionsController(mock, noopLogger{})

	c, _ := newParamContext(http.MethodPost, "/api/v1/collections", `{"name":"team"}`, "", "")
	cc.add(c)
	c, _ = newCollectionTermParamContext(http.MethodPost, "/api/v1/collections/team/terms/thing", "", "name", "team", "term", "thing")
	cc.addTerm(c)

	c, w := newCollectionTermParamContext(http.MethodDelete, "/api/v1/collections/team/terms/thing", "", "name", "team", "term", "thing")
	cc.removeTerm(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := mock.collections["team"].Terms; len(got) != 0 {
		t.Fatalf("Terms = %v, want empty after removeTerm()", got)
	}

	// Idempotent.
	c, w = newCollectionTermParamContext(http.MethodDelete, "/api/v1/collections/team/terms/thing", "", "name", "team", "term", "thing")
	cc.removeTerm(c)
	if w.Code != http.StatusOK {
		t.Fatalf("second removeTerm() status = %d, want %d (idempotent)", w.Code, http.StatusOK)
	}
}
