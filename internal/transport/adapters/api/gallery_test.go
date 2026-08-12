package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newMultipartAddRequest builds a real multipart/form-data request body
// (name field + a tiny valid PNG image field) — galleryController.add
// decodes the image for real (image.Decode), a JSON-body helper like
// newTestContext (recognition_test.go) can't exercise that path.
func newMultipartAddRequest(t *testing.T, name string, includeImage bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if name != "" {
		if err := mw.WriteField("name", name); err != nil {
			t.Fatalf("WriteField(name) error = %v", err)
		}
	}
	if includeImage {
		part, err := mw.CreateFormFile("image", "ref.png")
		if err != nil {
			t.Fatalf("CreateFormFile error = %v", err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		if err := png.Encode(part, img); err != nil {
			t.Fatalf("png.Encode error = %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart Close error = %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gallery", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.Request = req
	return c, w
}

// newParamContext mirrors newTestContext (recognition_test.go) but also
// sets a URL param — galleryController.remove/update read c.Param("name"),
// which gin only populates via real router matching, not
// gin.CreateTestContext alone.
func newParamContext(method, path, body, paramName, paramValue string) (*gin.Context, *httptest.ResponseRecorder) {
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
	c.Params = gin.Params{{Key: paramName, Value: paramValue}}
	return c, w
}

func TestGalleryAdd_ValidRequest_ReturnsCreated(t *testing.T) {
	mock := &mockUseCases{}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newMultipartAddRequest(t, "mon_sac", true)
	gc.add(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
	if _, ok := mock.galleryEntries["mon_sac"]; !ok {
		t.Fatal("gallery entry not recorded by the mock after add()")
	}
}

func TestGalleryAdd_MissingName_ReturnsBadRequest(t *testing.T) {
	mock := &mockUseCases{}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newMultipartAddRequest(t, "", true)
	gc.add(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGalleryAdd_MissingImage_ReturnsBadRequest(t *testing.T) {
	mock := &mockUseCases{}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newMultipartAddRequest(t, "mon_sac", false)
	gc.add(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGalleryAdd_UseCaseErrorSurfaced(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"mon_sac": true}} // pre-existing -> AddGalleryReference errors on duplicate
	gc := newGalleryController(mock, noopLogger{})

	c, w := newMultipartAddRequest(t, "mon_sac", true)
	gc.add(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGalleryList_ReturnsEntries(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"a": true, "b": false}}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newParamContext(http.MethodGet, "/api/v1/gallery", "", "", "")
	gc.list(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Entries []struct {
			Name    string `json:"Name"`
			Enabled bool   `json:"Enabled"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body.Entries) != 2 {
		t.Fatalf("entries = %+v, want 2", body.Entries)
	}
}

func TestGalleryRemove_IsIdempotent(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"mon_sac": true}}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newParamContext(http.MethodDelete, "/api/v1/gallery/mon_sac", "", "name", "mon_sac")
	gc.remove(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if _, ok := mock.galleryEntries["mon_sac"]; ok {
		t.Fatal("entry still present after remove()")
	}

	// Second delete of an already-gone name — still 200, not an error.
	c2, w2 := newParamContext(http.MethodDelete, "/api/v1/gallery/mon_sac", "", "name", "mon_sac")
	gc.remove(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second remove() status = %d, want %d (idempotent)", w2.Code, http.StatusOK)
	}
}

func TestGalleryUpdate_RenameAndToggle(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"old_name": true}}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newParamContext(http.MethodPatch, "/api/v1/gallery/old_name", `{"new_name":"new_name","enabled":false}`, "name", "old_name")
	gc.update(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	enabled, ok := mock.galleryEntries["new_name"]
	if !ok {
		t.Fatal("renamed entry not found")
	}
	if enabled {
		t.Fatal("entry should be disabled after update()")
	}
	if _, stillOld := mock.galleryEntries["old_name"]; stillOld {
		t.Fatal("old name should no longer resolve after rename")
	}
}

func TestGalleryUpdate_UseCaseErrorSurfaced(t *testing.T) {
	mock := &mockUseCases{}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newParamContext(http.MethodPatch, "/api/v1/gallery/nope", `{"enabled":true}`, "name", "nope")
	gc.update(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (unknown entry)", w.Code, http.StatusBadRequest)
	}
}

func TestGalleryUpdate_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	mock := &mockUseCases{galleryEntries: map[string]bool{"thing": true}}
	gc := newGalleryController(mock, noopLogger{})

	c, w := newParamContext(http.MethodPatch, "/api/v1/gallery/thing", `not json`, "name", "thing")
	gc.update(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
