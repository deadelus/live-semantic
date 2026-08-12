package uc

import (
	"image"

	"live-semantic/internal/domain/entities"
)

// AddGalleryReference encodes crop once (CLIP EncodeImage — the same call
// reanchor's semantic pass already makes per candidate box, TODO.md § A)
// and stores the resulting embedding under name. See UseCases' doc
// comment / ReferenceGallery.Add for the validation rules.
func (uc *UseCase) AddGalleryReference(name string, crop image.Image) error {
	embedding, err := uc.semanticEncoder.EncodeImage(&entities.Frame{Image: crop})
	if err != nil {
		return err
	}
	return uc.gallery.Add(name, embedding)
}

// RemoveGalleryReference — see ReferenceGallery.Remove.
func (uc *UseCase) RemoveGalleryReference(name string) {
	uc.gallery.Remove(name)
}

// RenameGalleryReference — see ReferenceGallery.Rename.
func (uc *UseCase) RenameGalleryReference(oldName, newName string) error {
	return uc.gallery.Rename(oldName, newName)
}

// SetGalleryReferenceEnabled — see ReferenceGallery.SetEnabled.
func (uc *UseCase) SetGalleryReferenceEnabled(name string, enabled bool) error {
	return uc.gallery.SetEnabled(name, enabled)
}

// ListGalleryReferences — see ReferenceGallery.List.
func (uc *UseCase) ListGalleryReferences() []GalleryEntryInfo {
	return uc.gallery.List()
}
