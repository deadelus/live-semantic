package api

import (
	"image"
	// Registered for their side effect (image.Decode format detection) —
	// the gallery accepts whatever common format a browser/GUI naturally
	// produces, not just JPEG like /ws/ingest's own decode path.
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// galleryController wires REST CRUD around uc.GalleryReferences (TODO.md
// § D/§ H1, docs/adr/clip-backend.md § 24) — a "recognize by reference
// image" filter term family, alongside COCO labels and free text. H1
// minimal scope, same as recognitionController: no auth, no per-session
// scoping (the gallery is process-wide, TODO.md § H1 "Multi-flux" doesn't
// change that — see infrastructure/storage.GalleryStorage's doc comment for
// why sharing across sessions is the point, not a limitation). Depends
// on uc.GalleryReferences, not the wider uc.UseCases (interface
// segregation, 2026-08-12) — this controller never touches recognition
// control.
type galleryController struct {
	useCases uc.GalleryReferences
	logger   logger.Logger
}

func newGalleryController(useCases uc.GalleryReferences, logger logger.Logger) *galleryController {
	return &galleryController{useCases: useCases, logger: logger}
}

// add handles POST /api/v1/gallery — multipart/form-data with a "name"
// field and an "image" file field, e.g.:
//
//	curl -F name=mon_sac -F image=@sac.jpg http://localhost:8080/api/v1/gallery
//
// Decodes whatever common image format the client sent (image.Decode
// auto-detects — JPEG/PNG registered above), not JPEG-only like
// /ws/ingest's own decode path, since a GUI's "select a box, submit a
// crop" flow isn't tied to the video pipeline's own encoding choice.
func (gc *galleryController) add(c *gin.Context) {
	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing form field \"name\""})
		return
	}

	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing form file \"image\": " + err.Error()})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open uploaded image: " + err.Error()})
		return
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to decode uploaded image: " + err.Error()})
		return
	}

	if err := gc.useCases.AddGalleryReference(c.Request.Context(), name, img); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "added", "name": name})
}

// list handles GET /api/v1/gallery.
func (gc *galleryController) list(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"entries": gc.useCases.ListGalleryReferences(c.Request.Context())})
}

// remove handles DELETE /api/v1/gallery/:name. Idempotent (matches
// storage.GalleryStorage.Remove) — always 200, even if the name never existed.
func (gc *galleryController) remove(c *gin.Context) {
	gc.useCases.RemoveGalleryReference(c.Request.Context(), c.Param("name"))
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// update handles PATCH /api/v1/gallery/:name — JSON body with either or
// both of "new_name" (rename) and "enabled" (toggle), e.g.
// {"enabled": false} or {"new_name": "autre_nom"}. Applied in that order
// (rename first) so a single call can do both.
func (gc *galleryController) update(c *gin.Context) {
	var body struct {
		NewName *string `json:"new_name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name := c.Param("name")
	if body.NewName != nil {
		if err := gc.useCases.RenameGalleryReference(c.Request.Context(), name, *body.NewName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		name = *body.NewName
	}
	if body.Enabled != nil {
		if err := gc.useCases.SetGalleryReferenceEnabled(c.Request.Context(), name, *body.Enabled); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated", "name": name})
}
