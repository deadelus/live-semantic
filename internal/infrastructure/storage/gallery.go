// Package storage holds storage ports for the application layer —
// currently just GalleryStorage, for "recognize by reference image"
// filter terms (docs/adr/clip-backend.md § 24), one or more named CLIP
// embeddings matched via image↔image cosine similarity instead of
// text↔image. Extracted 2026-08-12 from a concrete struct application/uc
// held directly (*ReferenceGallery) into a proper port +
// implementation/storage/inmemory adapter, following the same
// dependency-inversion shape as every other port in infrastructure/
// (notifier.AlertSender, inference.ObjectDetector,
// streamer.InputStream/OutputStream) — application/uc.UseCase now depends
// on GalleryStorage, not a concrete storage type. Renamed from package
// gallery/interface Repository (2026-08-12, same day) — package named
// after *what it is* (storage), interface named after *what it stores*
// (GalleryStorage), rather than a generic "Repository" that says nothing
// about its content and would collide in meaning if this package ever
// gains a second storage port for something other than the gallery.
//
// Multi-image + persistence — revised 2026-08-13, from a real product
// discussion: a single reference photo per named entry is fragile (one
// angle/lighting), and everything lived in RAM only (gone on restart,
// gone on crash) with no way to ever redisplay the photo a name was
// created from (only the embedding was kept, the crop itself discarded
// right after encoding). One entry now holds 0..N GalleryImage — several
// reference photos reinforcing the same name, each contributing its own
// embedding to matching (application/uc.tracking.go takes the best score
// across all of them, not just one) — and an implementation is expected
// to persist across restarts and keep the photo (or at least a
// thumbnail) so it can be shown back in a GUI, not just the vector. See
// implementation/storage/diskgallery for the disk-backed adapter that
// does both.
package storage

import "live-semantic/internal/domain/entities"

// GalleryImage is one reference photo within a Gallery entry. ID is
// storage-assigned (not the caller's choice) — stable and unique within
// its entry, needed so RemoveImage can delete exactly one photo among
// several without relying on array position (fragile: a concurrent
// Add/Remove could shift indices out from under a caller holding a
// stale one).
type GalleryImage struct {
	ID        string
	Embedding entities.Embedding
}

// Gallery is one stored reference entry, GalleryStorage's own view.
// Images is metadata only (ID + embedding) — thumbnail bytes are
// deliberately not inlined here (see GalleryStorage.Thumbnail) so List
// doesn't have to move image bytes around for callers that only want
// names/counts (e.g. the REST list endpoint) — a client fetches a
// specific thumbnail on demand, same shape as a normal image URL.
type Gallery struct {
	Name    string
	Enabled bool
	Images  []GalleryImage
	// CocoClass is an optional link to one of YOLO11s's 80 native COCO
	// classes (empty string means unlinked) — restricts this Term's
	// matching to boxes YOLO already classified into that class instead
	// of comparing against every detected box, per the Bibliothèque model
	// fixed in the 2026-08-13 mockups (docs/gui/design-brief.md §
	// Bibliothèque, screens 4c/4d). Business validation (must be one of
	// the 80 real class names) is the caller's job
	// (application/uc.UseCase.SetGalleryCocoClass) — same division as the
	// rest of this port, GalleryStorage itself doesn't know what a COCO
	// class is.
	CocoClass string
}

// GalleryStorage persists named, multi-image CLIP reference entries.
// Deliberately pure storage — name canonicalization (case/whitespace) is
// a legitimate storage-layer concern and stays here, but *business*
// rules about which names are valid (a name colliding with a COCO class
// would never be reachable as a filter term —
// application/uc.newTrackManager checks COCO labels first,
// unconditionally) are this port's caller's job
// (application/uc.UseCase.AddGalleryReference/RenameGalleryReference),
// not this interface's — GalleryStorage shouldn't need to know what a
// COCO class is. Thread-safety is each implementation's own concern.
type GalleryStorage interface {
	// AddImage adds one reference photo under name — creates a new,
	// enabled entry if name doesn't exist yet, or appends to an existing
	// one otherwise (an existing entry's Enabled state is untouched by an
	// append — no implicit re-enable). Errors if name is empty, embedding
	// is empty, or thumbnail is empty. Returns the new image's
	// storage-assigned ID.
	AddImage(name string, embedding entities.Embedding, thumbnail []byte) (imageID string, err error)
	// RemoveImage deletes one image by ID from name's entry — idempotent
	// (not an error if name or imageID doesn't exist, same convention as
	// Remove). If imageID was the entry's last remaining image, the whole
	// entry is removed too — an entry with zero reference images can
	// never match anything, so keeping an empty shell around would just
	// be a silent dead end the next time something tries to reuse that
	// name.
	RemoveImage(name, imageID string)
	// Remove deletes name's entire entry, every image (and thumbnail)
	// included — idempotent, same convention as HTTP DELETE.
	Remove(name string)
	// Rename changes an entry's key atomically, every image carried over
	// unchanged. Errors if oldName doesn't exist, newName is empty, or
	// newName already exists.
	Rename(oldName, newName string) error
	// SetEnabled toggles an entry (every image at once) without deleting
	// it. Errors if name doesn't exist.
	SetEnabled(name string, enabled bool) error
	// SetCocoClass sets or clears (cocoClass == "") name's linked COCO
	// class — see Gallery.CocoClass's doc comment. Errors if name doesn't
	// exist. Doesn't validate cocoClass is actually one of the 80 real
	// class names — that's the caller's job (see this interface's own doc
	// comment on business validation living outside this port).
	SetCocoClass(name, cocoClass string) error
	// Get returns every embedding of name's images — ok is false if the
	// entry doesn't exist, is disabled, or has zero images (a disabled or
	// image-less entry must behave as absent to a filter-term lookup).
	Get(name string) ([]entities.Embedding, bool)
	// Thumbnail returns one image's stored thumbnail bytes (JPEG) — ok is
	// false if name or imageID doesn't exist.
	Thumbnail(name, imageID string) ([]byte, bool)
	// List returns every entry (enabled or not), sorted by name.
	List() []Gallery
}
