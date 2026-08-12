// Package gallery is the port for storing "recognize by reference image"
// filter terms (TODO.md § D "reconnaissance par référence image" / § H1,
// docs/adr/clip-backend.md § 24) — a named CLIP embedding, matched via
// image↔image cosine similarity instead of text↔image. Extracted
// 2026-08-12 from a concrete struct application/uc held directly
// (*ReferenceGallery) into a proper port + implementation/gallery/inmemory
// adapter, following the same dependency-inversion shape as every other
// port in infrastructure/ (notifier.AlertSender, inference.ObjectDetector,
// streamer.InputStream/OutputStream) — application/uc.UseCase now depends
// on Repository, not a concrete storage type.
package gallery

import "live-semantic/internal/domain/entities"

// Entry is one stored reference, Repository's own view (includes the
// embedding — application/uc.GalleryEntryInfo is the embedding-free
// client-facing projection of this, derived by the use case layer, not
// this port).
type Entry struct {
	Name      string
	Embedding entities.Embedding
	Enabled   bool
}

// Repository persists named embeddings. Deliberately pure storage — name
// canonicalization (case/whitespace) is a legitimate storage-layer
// concern and stays here, but *business* rules about which names are
// valid (a name colliding with a COCO class would never be reachable as
// a filter term — application/uc.newTrackManager checks COCO labels
// first, unconditionally) are this port's caller's job
// (application/uc.UseCase.AddGalleryReference/RenameGalleryReference),
// not this interface's — a Repository shouldn't need to know what a COCO
// class is. Add/Rename still reject a name that already exists in
// storage (a uniqueness constraint, same category of invariant as a DB's
// unique index — legitimately this layer's concern, not a business
// rule). Thread-safety is each implementation's own concern — see
// implementation/gallery/inmemory for the only one that exists today.
type Repository interface {
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
	List() []Entry
}
