package uc

import (
	"context"
	"fmt"
	"image"

	"live-semantic/internal/domain/entities"
)

// GalleryEntryInfo is the read-only, embedding-free view of a gallery
// entry returned to a REST caller — embedding vectors (512 floats for
// CLIP ViT-B/32) have no use client-side and shouldn't cross that
// boundary. Derived from gallery.Entry (infrastructure/gallery's own,
// embedding-included view) by ListGalleryReferences below.
type GalleryEntryInfo struct {
	Name    string
	Enabled bool
}

// AddGalleryReference validates name (business rules: can't be empty,
// can't collide with a COCO class — newTrackManager checks COCO labels
// first, unconditionally, so a gallery entry sharing a COCO name would
// silently never be reachable as a filter term), then encodes crop once
// (CLIP EncodeImage — the same call reanchor's semantic pass already
// makes per candidate box, TODO.md § A) and stores the resulting
// embedding under name via uc.gallery (the gallery.Repository port —
// pure storage, doesn't know what a COCO class is, see
// infrastructure/gallery.Repository's doc comment for why that split
// exists). ctx is checked before EncodeImage runs — fail fast on an
// already-cancelled request rather than paying for a CGo/ONNX inference
// call whose result nobody will read (same "check before I/O" rationale
// as RecognitionUseCase's filter-parse-before-camera-open,
// uc_recognition.go).
func (uc *UseCase) AddGalleryReference(ctx context.Context, name string, crop image.Image) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	name = normalizeFilter(name)
	if name == "" {
		return fmt.Errorf("gallery entry name cannot be empty")
	}
	if isCOCOLabel(name) {
		return fmt.Errorf("gallery entry name %q collides with a COCO class — it would never be reachable as a filter term (COCO labels are checked first)", name)
	}

	embedding, err := uc.semanticEncoder.EncodeImage(&entities.Frame{Image: crop})
	if err != nil {
		return err
	}
	return uc.gallery.Add(name, embedding)
}

// RemoveGalleryReference — see gallery.Repository.Remove. ctx accepted
// for interface consistency (see AddGalleryReference's doc comment) —
// the underlying delete is effectively instantaneous, nothing to
// meaningfully cancel, so ctx isn't checked here.
func (uc *UseCase) RemoveGalleryReference(_ context.Context, name string) {
	uc.gallery.Remove(name)
}

// RenameGalleryReference validates newName the same way
// AddGalleryReference validates name (empty/COCO-collision — see its
// doc comment), then delegates to uc.gallery.Rename for the atomic
// storage-level swap (missing source / target-already-exists errors).
func (uc *UseCase) RenameGalleryReference(_ context.Context, oldName, newName string) error {
	newName = normalizeFilter(newName)
	if newName == "" {
		return fmt.Errorf("new gallery entry name cannot be empty")
	}
	if isCOCOLabel(newName) {
		return fmt.Errorf("gallery entry name %q collides with a COCO class — it would never be reachable as a filter term", newName)
	}
	return uc.gallery.Rename(oldName, newName)
}

// SetGalleryReferenceEnabled — see gallery.Repository.SetEnabled. ctx:
// see RemoveGalleryReference's doc comment.
func (uc *UseCase) SetGalleryReferenceEnabled(_ context.Context, name string, enabled bool) error {
	return uc.gallery.SetEnabled(name, enabled)
}

// ListGalleryReferences — see gallery.Repository.List. Projects each
// gallery.Entry down to the embedding-free GalleryEntryInfo (see its own
// doc comment for why). ctx: see RemoveGalleryReference's doc comment.
func (uc *UseCase) ListGalleryReferences(_ context.Context) []GalleryEntryInfo {
	entries := uc.gallery.List()
	out := make([]GalleryEntryInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, GalleryEntryInfo{Name: e.Name, Enabled: e.Enabled})
	}
	return out
}
