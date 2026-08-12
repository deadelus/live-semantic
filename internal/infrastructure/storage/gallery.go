// Package storage holds storage ports for the application layer —
// currently just GalleryStorage, for "recognize by reference image"
// filter terms (TODO.md § D "reconnaissance par référence image" / § H1,
// docs/adr/clip-backend.md § 24), a named CLIP embedding matched via
// image↔image cosine similarity instead of text↔image. Extracted
// 2026-08-12 from a concrete struct application/uc held directly
// (*ReferenceGallery) into a proper port + implementation/storage/inmemory
// adapter, following the same dependency-inversion shape as every other
// port in infrastructure/ (notifier.AlertSender, inference.ObjectDetector,
// streamer.InputStream/OutputStream) — application/uc.UseCase now depends
// on GalleryStorage, not a concrete storage type. Renamed from package
// gallery/interface Repository (2026-08-12, same day) — package named
// after *what it is* (storage), interface named after *what it stores*
// (GalleryStorage), rather than a generic "Repository" that says nothing
// about its content and would collide in meaning if this package ever
// gains a second storage port for something other than the gallery.
package storage

import "live-semantic/internal/domain/entities"

// Gallery is one stored reference, GalleryStorage's own view (includes
// the embedding — application/uc.GalleryEntryInfo is the embedding-free
// client-facing projection of this, derived by the use case layer, not
// this port).
type Gallery struct {
	Name      string
	Embedding entities.Embedding
	Enabled   bool
}

// GalleryStorage persists named embeddings. Deliberately pure storage —
// name canonicalization (case/whitespace) is a legitimate storage-layer
// concern and stays here, but *business* rules about which names are
// valid (a name colliding with a COCO class would never be reachable as
// a filter term — application/uc.newTrackManager checks COCO labels
// first, unconditionally) are this port's caller's job
// (application/uc.UseCase.AddGalleryReference/RenameGalleryReference),
// not this interface's — GalleryStorage shouldn't need to know what a
// COCO class is. Add/Rename still reject a name that already exists in
// storage (a uniqueness constraint, same category of invariant as a DB's
// unique index — legitimately this layer's concern, not a business
// rule). Thread-safety is each implementation's own concern — see
// implementation/storage/inmemory for the only one that exists today.
type GalleryStorage interface {
	// Add stores a new entry, enabled by default. Errors if name is empty,
	// embedding is empty, or name already exists.
	Add(name string, embedding entities.Embedding) error
	// Remove deletes an entry — not an error if it doesn't exist (same
	// idempotent-delete convention as HTTP DELETE).
	Remove(name string)
	// Rename changes an entry's key atomically. Errors if oldName doesn't
	// exist, newName is empty, or newName already exists.
	Rename(oldName, newName string) error
	// SetEnabled toggles an entry without deleting it. Errors if name
	// doesn't exist.
	SetEnabled(name string, enabled bool) error
	// Get returns name's embedding — ok is false if the entry doesn't
	// exist OR is disabled (a disabled entry must behave as absent to a
	// filter-term lookup).
	Get(name string) (entities.Embedding, bool)
	// List returns every entry (enabled or not), sorted by name.
	List() []Gallery
}
