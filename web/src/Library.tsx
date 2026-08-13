import { useEffect, useState } from 'react'
import {
  addTermToCollection,
  createCollection,
  listCollections,
  listGallery,
  removeCollection,
  removeGalleryTerm,
  removeTermFromCollection,
  updateCollection,
  updateGalleryTerm,
  type CollectionEntry,
  type GalleryEntry,
} from './api'
import { Button } from './components/Button'
import { useToast } from './toast/ToastProvider'
import './Library.css'

// Bibliothèque screen — Collections (docs/gui/mockups/ screens 4a/4b) +
// Termes (screen 4c/4d's underlying data, minus the photo-upload modal —
// see the note at the bottom of this file for why). Backend added
// 2026-08-13 (branch feat/gui-library-backend): storage.CollectionStorage
// + storage.Gallery.CocoClass, exposed via /api/v1/collections and
// PATCH .../gallery/:name's coco_class field.
//
// Deliberately NOT built here: the "+ Nouveau Terme" photo-upload modal
// (screen 4c) — creating a Term requires at least one reference photo
// (storage.GalleryStorage.AddImage, "un Terme sans photo n'existe pas"),
// and the only way to produce one today is POST /api/v1/gallery's
// multipart upload, which needs a source image. The intended real source
// (clicking/drawing a box on the live video, TODO.md § H1 "Sélection
// runtime + labellisation") isn't built yet — bolting a bare file-picker
// onto this screen instead would be a disconnected, throwaway upload
// flow, not the actual product interaction. This screen therefore
// manages Terms that already exist (rename, enable/disable, link a COCO
// class, delete, group into Collections) — full CRUD once a Term exists,
// just not creation.
export function Library() {
  const [collections, setCollections] = useState<CollectionEntry[]>([])
  const [terms, setTerms] = useState<GalleryEntry[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [newCollectionName, setNewCollectionName] = useState('')
  const [addTermName, setAddTermName] = useState('')
  const { pushError } = useToast()

  const reload = () => {
    listCollections()
      .then((r) => setCollections(r.collections))
      .catch(() => pushError('Impossible de charger les Collections.'))
    listGallery()
      .then((r) => setTerms(r.entries))
      .catch(() => pushError('Impossible de charger les Termes.'))
  }

  useEffect(reload, []) // eslint-disable-line react-hooks/exhaustive-deps

  const selectedCollection = collections.find((c) => c.name === selected) ?? null

  const handleCreateCollection = async () => {
    const name = newCollectionName.trim()
    if (!name) return
    try {
      await createCollection(name, [])
      setNewCollectionName('')
      reload()
    } catch {
      pushError('Impossible de créer cette Collection (nom déjà utilisé ?).')
    }
  }

  const handleDeleteCollection = async (name: string) => {
    try {
      await removeCollection(name)
      if (selected === name) setSelected(null)
      reload()
    } catch {
      pushError('Impossible de supprimer cette Collection.')
    }
  }

  const handleSetTags = async (name: string, tagsCsv: string) => {
    const tags = tagsCsv
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
    try {
      await updateCollection(name, { tags })
      reload()
    } catch {
      pushError('Impossible de mettre à jour les tags.')
    }
  }

  const handleAddTerm = async (collectionName: string) => {
    const term = addTermName.trim()
    if (!term) return
    try {
      await addTermToCollection(collectionName, term)
      setAddTermName('')
      reload()
    } catch {
      pushError("Ce Terme n'existe pas dans la Bibliothèque (ou est déjà dans cette Collection).")
    }
  }

  const handleRemoveTerm = async (collectionName: string, term: string) => {
    try {
      await removeTermFromCollection(collectionName, term)
      reload()
    } catch {
      pushError('Impossible de retirer ce Terme de la Collection.')
    }
  }

  const handleToggleTerm = async (term: GalleryEntry) => {
    try {
      await updateGalleryTerm(term.name, { enabled: !term.enabled })
      reload()
    } catch {
      pushError('Impossible de mettre à jour ce Terme.')
    }
  }

  const handleSetCocoClass = async (name: string, cocoClass: string) => {
    try {
      await updateGalleryTerm(name, { cocoClass: cocoClass.trim() })
      reload()
    } catch {
      pushError("Cette classe COCO n'existe pas dans le modèle actif.")
    }
  }

  const handleDeleteTerm = async (name: string) => {
    try {
      await removeGalleryTerm(name)
      reload()
    } catch {
      pushError('Impossible de supprimer ce Terme.')
    }
  }

  return (
    <div className="ls-library">
      <section className="ls-library__section">
        <div className="ls-library__section-header">
          <h2>Collections</h2>
          <div className="ls-library__inline-form">
            <input
              className="ls-library__input"
              placeholder="Nom de la Collection…"
              value={newCollectionName}
              onChange={(e) => setNewCollectionName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleCreateCollection()}
            />
            <Button variant="primary" onClick={handleCreateCollection}>
              + Nouvelle Collection
            </Button>
          </div>
        </div>

        {collections.length === 0 ? (
          <p className="ls-muted">Aucune Collection pour l'instant.</p>
        ) : (
          <div className="ls-library__cards">
            {collections.map((c) => (
              <button
                key={c.name}
                className={`ls-library__card ${selected === c.name ? 'ls-library__card--selected' : ''}`}
                onClick={() => setSelected(selected === c.name ? null : c.name)}
              >
                <span className="ls-library__card-name">{c.name}</span>
                <span className="ls-library__card-count">{c.terms.length} Termes</span>
                <div className="ls-library__tags">
                  {c.tags.map((t) => (
                    <span key={t} className="ls-library__tag">
                      {t}
                    </span>
                  ))}
                </div>
              </button>
            ))}
          </div>
        )}

        {selectedCollection && (
          <div className="ls-library__detail">
            <div className="ls-library__detail-header">
              <span className="ls-library__detail-title">{selectedCollection.name}</span>
              <Button variant="danger" onClick={() => handleDeleteCollection(selectedCollection.name)}>
                Supprimer la Collection
              </Button>
            </div>

            <label className="ls-library__field">
              <span>Tags (séparés par des virgules)</span>
              <input
                className="ls-library__input"
                defaultValue={selectedCollection.tags.join(', ')}
                onBlur={(e) => handleSetTags(selectedCollection.name, e.target.value)}
              />
            </label>

            <ul className="ls-library__term-list">
              {selectedCollection.terms.length === 0 ? (
                <li className="ls-muted">Aucun Terme pour l'instant.</li>
              ) : (
                selectedCollection.terms.map((t) => (
                  <li key={t} className="ls-library__term-row">
                    <span>{t}</span>
                    <Button
                      variant="discrete"
                      onClick={() => handleRemoveTerm(selectedCollection.name, t)}
                    >
                      Retirer
                    </Button>
                  </li>
                ))
              )}
            </ul>

            <div className="ls-library__inline-form">
              <input
                className="ls-library__input"
                placeholder="Nom d'un Terme existant…"
                value={addTermName}
                onChange={(e) => setAddTermName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleAddTerm(selectedCollection.name)}
              />
              <Button variant="secondary" onClick={() => handleAddTerm(selectedCollection.name)}>
                Ajouter à la Collection
              </Button>
            </div>
            <p className="ls-muted">
              Retirer un Terme ne le supprime pas de la Bibliothèque.
            </p>
          </div>
        )}
      </section>

      <section className="ls-library__section">
        <h2>Tous les Termes</h2>
        {terms.length === 0 ? (
          <p className="ls-muted">
            Aucun Terme pour l'instant — un Terme se crée depuis la Vue live en sélectionnant un
            objet (fonctionnalité pas encore construite, TODO.md § H1 « Sélection runtime »).
          </p>
        ) : (
          <ul className="ls-library__term-list ls-library__term-list--full">
            {terms.map((t) => (
              <li key={t.name} className="ls-library__term-row">
                <span className="ls-library__term-name">{t.name}</span>
                <span className="ls-library__term-meta">
                  {t.images.length} photo{t.images.length > 1 ? 's' : ''}
                </span>
                <input
                  className="ls-library__input ls-library__input--small"
                  placeholder="classe COCO (optionnel)"
                  defaultValue={t.cocoClass ?? ''}
                  onBlur={(e) => handleSetCocoClass(t.name, e.target.value)}
                />
                <Button variant={t.enabled ? 'secondary' : 'neutral'} onClick={() => handleToggleTerm(t)}>
                  {t.enabled ? 'Activé' : 'Désactivé'}
                </Button>
                <Button variant="danger" onClick={() => handleDeleteTerm(t.name)}>
                  Supprimer
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
