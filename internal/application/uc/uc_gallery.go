package uc

import (
	"context"
	"image"

	"live-semantic/internal/domain/entities"
)

// AddGalleryReference encodes crop once (CLIP EncodeImage — the same call
// reanchor's semantic pass already makes per candidate box, TODO.md § A)
// and stores the resulting embedding under name. See UseCases' doc
// comment / ReferenceGallery.Add for the validation rules. ctx is checked
// before EncodeImage runs — fail fast on an already-cancelled request
// rather than paying for a CGo/ONNX inference call whose result nobody
// will read (same "check before I/O" rationale as RecognitionUseCase's
// filter-parse-before-camera-open, uc_recognition.go).
func (uc *UseCase) AddGalleryReference(ctx context.Context, name string, crop image.Image) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	embedding, err := uc.semanticEncoder.EncodeImage(&entities.Frame{Image: crop})
	if err != nil {
		return err
	}
	return uc.gallery.Add(name, embedding)
}

// RemoveGalleryReference — see ReferenceGallery.Remove. ctx accepted for
// interface consistency (see UseCases.AddGalleryReference's doc comment)
// — the underlying map delete is effectively instantaneous, nothing to
// meaningfully cancel, so ctx isn't checked here.
func (uc *UseCase) RemoveGalleryReference(_ context.Context, name string) {
	uc.gallery.Remove(name)
}

// RenameGalleryReference — see ReferenceGallery.Rename. ctx: see
// RemoveGalleryReference's doc comment.
func (uc *UseCase) RenameGalleryReference(_ context.Context, oldName, newName string) error {
	return uc.gallery.Rename(oldName, newName)
}

// SetGalleryReferenceEnabled — see ReferenceGallery.SetEnabled. ctx: see
// RemoveGalleryReference's doc comment.
func (uc *UseCase) SetGalleryReferenceEnabled(_ context.Context, name string, enabled bool) error {
	return uc.gallery.SetEnabled(name, enabled)
}

// ListGalleryReferences — see ReferenceGallery.List. ctx: see
// RemoveGalleryReference's doc comment.
func (uc *UseCase) ListGalleryReferences(_ context.Context) []GalleryEntryInfo {
	return uc.gallery.List()
}
