package storage

// Collection groups Terms (Gallery entries, see gallery.go) under a name
// with flat tags for cross-cutting filtering — added 2026-08-13 alongside
// Gallery.CocoClass, both pieces of the Bibliothèque model fixed by the
// 4a-4e mockups (docs/gui/design-brief.md § Bibliothèque). "Flat" is
// load-bearing: a Collection never nests inside another one, only tags
// do the cross-cutting (screen 4a's tag filter).
//
// A Collection *references* Terms by name, it never owns or copies their
// data — the same Term can belong to any number of Collections at once
// (screen 4c's "Dans : Manchester Team, Adversaires"), and deleting a
// Collection never deletes the Terms it referenced (only the grouping).
// This mirrors GalleryStorage's own storage-only scope: CollectionStorage
// doesn't verify a referenced term name actually exists in
// GalleryStorage — that cross-port consistency check is
// application/uc.UseCase's job (it holds both ports), not this one's.
type Collection struct {
	Name  string
	Tags  []string
	Terms []string // Gallery entry names, referenced not owned
}

// CollectionStorage persists Collections. Thread-safety is each
// implementation's own concern, same convention as GalleryStorage.
type CollectionStorage interface {
	// Create adds a new, empty (zero Terms) Collection. Errors if name is
	// empty or a Collection by that name already exists.
	Create(name string, tags []string) error
	// Delete removes a Collection — idempotent (not an error if absent,
	// same convention as GalleryStorage.Remove). Never touches the Terms
	// it referenced.
	Delete(name string)
	// Rename changes a Collection's key, tags and Terms carried over
	// unchanged. Errors if oldName doesn't exist, newName is empty, or
	// newName already exists.
	Rename(oldName, newName string) error
	// SetTags replaces a Collection's tag set wholesale. Errors if name
	// doesn't exist.
	SetTags(name string, tags []string) error
	// AddTerm adds termName to a Collection's Terms — idempotent, no
	// duplicate entry if termName is already listed. Errors if the
	// Collection doesn't exist.
	AddTerm(collectionName, termName string) error
	// RemoveTerm removes termName from a Collection's Terms — idempotent
	// (not an error if the Collection or the term reference doesn't
	// exist), same convention as GalleryStorage.RemoveImage.
	RemoveTerm(collectionName, termName string)
	// Get returns one Collection — ok is false if it doesn't exist.
	Get(name string) (Collection, bool)
	// List returns every Collection, sorted by name.
	List() []Collection
}
