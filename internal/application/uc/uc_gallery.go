package uc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"

	"live-semantic/internal/domain/entities"

	"github.com/nfnt/resize"
)

// GalleryImageInfo is the read-only, embedding-free view of one reference
// photo within a gallery entry — just enough (an ID) for a REST caller to
// build a thumbnail URL (GetGalleryThumbnail) and to target RemoveGalleryImage.
type GalleryImageInfo struct {
	ID string `json:"id"`
}

// GalleryEntryInfo is the read-only, embedding-free view of a gallery
// entry returned to a REST caller — embedding vectors (512 floats for
// CLIP ViT-B/32) have no use client-side and shouldn't cross that
// boundary. Derived from storage.Gallery (infrastructure/storage's own,
// embedding-included view) by ListGalleryReferences below. Images gained
// its own field 2026-08-13 alongside multi-image entries — a client
// needs to know how many photos an entry has and their IDs to render a
// thumbnail grid and offer per-photo removal (v2 of the delete flow,
// alongside whole-entry removal).
type GalleryEntryInfo struct {
	Name      string             `json:"name"`
	Enabled   bool               `json:"enabled"`
	Images    []GalleryImageInfo `json:"images"`
	CocoClass string             `json:"cocoClass,omitempty"`
}

// CollectionInfo is the read-only view of a Collection returned to a REST
// caller — a straight projection of storage.Collection, no
// embedding/vector data to strip out here (unlike GalleryEntryInfo),
// kept as its own type anyway for the same reason GalleryEntryInfo is:
// application/uc's public shape shouldn't just be a re-export of an
// infrastructure/storage type, so the two layers can evolve
// independently.
type CollectionInfo struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Terms []string `json:"terms"`
}

// thumbnailMaxDimension bounds a stored reference photo's thumbnail —
// small enough to be cheap to store/serve for a GUI grid of vignettes,
// large enough to still recognize the subject. Aspect ratio preserved
// (resize.Thumbnail, not resize.Resize — the latter would distort a
// non-square crop).
const thumbnailMaxDimension = 160

// thumbnailQuality mirrors implementation/streamer/output/websocket.go's
// own JPEG quality choice for outgoing video frames — same "good enough,
// not archival" trade-off applies here.
const thumbnailQuality = 80

// encodeThumbnail downscales crop to thumbnailMaxDimension and JPEG-
// encodes it — the same nfnt/resize + image/jpeg pair already used
// elsewhere in this codebase (inference input preprocessing,
// implementation/streamer/output's outgoing frames respectively), not a
// new pattern.
func encodeThumbnail(crop image.Image) ([]byte, error) {
	resized := resize.Thumbnail(thumbnailMaxDimension, thumbnailMaxDimension, crop, resize.Lanczos3)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), nil
}

// AddGalleryReference validates name (business rules: can't be empty,
// can't collide with a COCO class — newTrackManager checks COCO labels
// first, unconditionally, so a gallery entry sharing a COCO name would
// silently never be reachable as a filter term), then encodes crop once
// (CLIP EncodeImage — the same call reanchor's semantic pass already
// makes per candidate box) and stores the resulting embedding — plus a
// downscaled JPEG thumbnail of crop itself (2026-08-13: the raw crop used
// to be discarded right after encoding, making it structurally
// impossible to ever show the reference photo back in a GUI) — under
// name via uc.gallery (the storage.GalleryStorage port — pure storage,
// doesn't know what a COCO class is, see
// infrastructure/storage.GalleryStorage's doc comment for why that split
// exists). Calling this again with a name that already has entries
// *adds* another reference photo to it rather than erroring — see
// storage.GalleryStorage.AddImage's own doc comment; several photos of
// the same subject (different angle/lighting) make the CLIP image↔image
// match more robust than one fragile photo, application/uc/tracking.go's
// matching takes the best score across all of a term's images. ctx is
// checked before EncodeImage runs — fail fast on an already-cancelled
// request rather than paying for a CGo/ONNX inference call whose result
// nobody will read (same "check before I/O" rationale as
// RecognitionUseCase's filter-parse-before-camera-open, uc_recognition.go).
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
	thumbnail, err := encodeThumbnail(crop)
	if err != nil {
		return err
	}
	_, err = uc.gallery.AddImage(name, embedding, thumbnail)
	return err
}

// RemoveGalleryImage deletes one reference photo (by ID, as returned in
// GalleryEntryInfo.Images) from name's entry — see
// storage.GalleryStorage.RemoveImage's doc comment for the "last image
// removes the whole entry" behavior. ctx: see RemoveGalleryReference's
// doc comment.
func (uc *UseCase) RemoveGalleryImage(_ context.Context, name, imageID string) {
	uc.gallery.RemoveImage(name, imageID)
}

// GetGalleryThumbnail returns one reference photo's stored thumbnail
// bytes (JPEG) — for a REST handler to serve directly. ctx: see
// RemoveGalleryReference's doc comment.
func (uc *UseCase) GetGalleryThumbnail(_ context.Context, name, imageID string) ([]byte, bool) {
	return uc.gallery.Thumbnail(name, imageID)
}

// RemoveGalleryReference — see storage.GalleryStorage.Remove. ctx accepted
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

// SetGalleryReferenceEnabled — see storage.GalleryStorage.SetEnabled. ctx:
// see RemoveGalleryReference's doc comment.
func (uc *UseCase) SetGalleryReferenceEnabled(_ context.Context, name string, enabled bool) error {
	return uc.gallery.SetEnabled(name, enabled)
}

// ListGalleryReferences — see storage.GalleryStorage.List. Projects each
// storage.Gallery down to the embedding-free GalleryEntryInfo (see its own
// doc comment for why). ctx: see RemoveGalleryReference's doc comment.
func (uc *UseCase) ListGalleryReferences(_ context.Context) []GalleryEntryInfo {
	entries := uc.gallery.List()
	out := make([]GalleryEntryInfo, 0, len(entries))
	for _, e := range entries {
		images := make([]GalleryImageInfo, len(e.Images))
		for i, img := range e.Images {
			images[i] = GalleryImageInfo{ID: img.ID}
		}
		out = append(out, GalleryEntryInfo{Name: e.Name, Enabled: e.Enabled, Images: images, CocoClass: e.CocoClass})
	}
	return out
}

// SetGalleryCocoClass links name to cocoClass (the Bibliothèque model's
// optional "classe COCO liée", docs/gui/design-brief.md § Bibliothèque,
// screen 4c) — restricts this Term's matching to boxes already classified
// there by YOLO (application/uc/tracking.go). cocoClass == "" clears an
// existing link. A non-empty cocoClass must be one of the 80 real class
// names — screen 4d's "warning" state (no COCO class matches) is exactly
// this validation failing, surfaced to the caller as an error rather than
// silently storing an unmatchable class name.
func (uc *UseCase) SetGalleryCocoClass(_ context.Context, name, cocoClass string) error {
	if cocoClass != "" && !isCOCOLabel(cocoClass) {
		return fmt.Errorf("%q is not one of the 80 COCO classes of the active model", cocoClass)
	}
	return uc.gallery.SetCocoClass(name, cocoClass)
}

// --- Bibliothèque — Collections (2026-08-13). See use_case.go's
// GalleryReferences doc comments for why these live in the same
// interface/file as the Term (gallery) methods above. ---

// CreateCollection — see storage.CollectionStorage.Create. No name
// validation beyond what the storage layer already does (non-empty,
// unique) — unlike Term names, a Collection name never collides with a
// COCO class (it's not itself usable as a filter term, only the Terms it
// groups are).
func (uc *UseCase) CreateCollection(_ context.Context, name string, tags []string) error {
	return uc.collections.Create(name, tags)
}

// DeleteCollection — see storage.CollectionStorage.Delete.
func (uc *UseCase) DeleteCollection(_ context.Context, name string) {
	uc.collections.Delete(name)
}

// RenameCollection — see storage.CollectionStorage.Rename.
func (uc *UseCase) RenameCollection(_ context.Context, oldName, newName string) error {
	return uc.collections.Rename(oldName, newName)
}

// SetCollectionTags — see storage.CollectionStorage.SetTags.
func (uc *UseCase) SetCollectionTags(_ context.Context, name string, tags []string) error {
	return uc.collections.SetTags(name, tags)
}

// AddTermToCollection links termName into collectionName — validated here
// (not in storage.CollectionStorage, which deliberately doesn't know
// GalleryStorage exists, see its own doc comment) against the gallery:
// grouping a name that isn't a real Term yet would silently produce a
// Collection screen listing a term with no photos, which the Bibliothèque
// model says can't exist ("un Terme sans photo n'existe pas", screen 4a's
// model note) — reject it up front instead.
func (uc *UseCase) AddTermToCollection(_ context.Context, collectionName, termName string) error {
	if _, ok := uc.gallery.Get(termName); !ok {
		return fmt.Errorf("term %q does not exist (or has no reference photos) in the gallery", termName)
	}
	return uc.collections.AddTerm(collectionName, termName)
}

// RemoveTermFromCollection — see storage.CollectionStorage.RemoveTerm.
// Never deletes the Term itself, only the grouping (screen 4b: "Retirer
// un Terme ne le supprime pas de la bibliothèque").
func (uc *UseCase) RemoveTermFromCollection(_ context.Context, collectionName, termName string) {
	uc.collections.RemoveTerm(collectionName, termName)
}

// ListCollections — see storage.CollectionStorage.List.
func (uc *UseCase) ListCollections(_ context.Context) []CollectionInfo {
	collections := uc.collections.List()
	out := make([]CollectionInfo, 0, len(collections))
	for _, c := range collections {
		out = append(out, CollectionInfo{Name: c.Name, Tags: c.Tags, Terms: c.Terms})
	}
	return out
}
