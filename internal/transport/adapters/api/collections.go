package api

import (
	"net/http"

	"live-semantic/internal/application/uc"

	"github.com/deadelus/go-clean-app/v2/logger"
	"github.com/gin-gonic/gin"
)

// collectionsController wires REST CRUD around the Collections half of
// uc.GalleryReferences (docs/gui/design-brief.md § Bibliothèque,
// 2026-08-13) — a separate controller/REST resource
// (/api/v1/collections) from galleryController's /api/v1/gallery even
// though both depend on the very same uc.GalleryReferences interface
// (see that interface's own doc comment on why Collections methods live
// there rather than a narrower interface): a Collection and a Term are
// two distinct REST resources with their own CRUD verbs, bundling both
// under one controller/route prefix would blur that even though the
// application-layer interface doesn't need to.
type collectionsController struct {
	useCases uc.GalleryReferences
	logger   logger.Logger
}

func newCollectionsController(useCases uc.GalleryReferences, logger logger.Logger) *collectionsController {
	return &collectionsController{useCases: useCases, logger: logger}
}

// add handles POST /api/v1/collections — JSON body {"name": "...",
// "tags": ["..."]} (tags optional/omittable, an empty Collection can be
// created with zero tags and zero Terms, screen 4a's "+ Nouvelle
// Collection").
func (cc *collectionsController) add(c *gin.Context) {
	var body struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := cc.useCases.CreateCollection(c.Request.Context(), body.Name, body.Tags); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "created", "name": body.Name})
}

// list handles GET /api/v1/collections.
func (cc *collectionsController) list(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"collections": cc.useCases.ListCollections(c.Request.Context())})
}

// remove handles DELETE /api/v1/collections/:name — idempotent, matches
// storage.CollectionStorage.Delete. Never removes the Terms it
// referenced (screen 4b: "Retirer un Terme ne le supprime pas de la
// bibliothèque" — the inverse holds too, deleting the Collection doesn't
// touch its Terms).
func (cc *collectionsController) remove(c *gin.Context) {
	cc.useCases.DeleteCollection(c.Request.Context(), c.Param("name"))
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// update handles PATCH /api/v1/collections/:name — JSON body with either
// or both of "new_name" (rename) and "tags" (replace wholesale — the
// combobox in screen 4b always sends the full resulting tag set, not a
// diff). Applied in that order, same convention as galleryController.update.
func (cc *collectionsController) update(c *gin.Context) {
	var body struct {
		NewName *string  `json:"new_name"`
		Tags    []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name := c.Param("name")
	if body.NewName != nil {
		if err := cc.useCases.RenameCollection(c.Request.Context(), name, *body.NewName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		name = *body.NewName
	}
	if body.Tags != nil {
		if err := cc.useCases.SetCollectionTags(c.Request.Context(), name, body.Tags); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated", "name": name})
}

// addTerm handles POST /api/v1/collections/:name/terms/:term — links an
// existing Term into the Collection (screen 4c's "Dans : ..." footer).
// Errors (400) if :term isn't a real Term in the gallery yet — see
// uc.AddTermToCollection's doc comment.
func (cc *collectionsController) addTerm(c *gin.Context) {
	if err := cc.useCases.AddTermToCollection(c.Request.Context(), c.Param("name"), c.Param("term")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

// removeTerm handles DELETE /api/v1/collections/:name/terms/:term —
// idempotent, only removes the grouping (see uc.RemoveTermFromCollection's
// doc comment — the Term itself is untouched).
func (cc *collectionsController) removeTerm(c *gin.Context) {
	cc.useCases.RemoveTermFromCollection(c.Request.Context(), c.Param("name"), c.Param("term"))
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}
