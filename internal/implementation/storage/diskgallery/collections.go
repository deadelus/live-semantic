package diskgallery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"live-semantic/internal/infrastructure/storage"
)

// Collections implements storage.CollectionStorage against a single JSON
// file under root — added 2026-08-13 alongside Gallery.CocoClass, both
// pieces of the Bibliothèque model (docs/gui/design-brief.md §
// Bibliothèque). One file, not one-directory-per-collection like Gallery:
// a Collection is a small, cheap-to-rewrite-wholesale record (a name, a
// handful of tags, a list of term-name references) — none of Gallery's
// reasons for per-entry directories (large thumbnail files, an atomic
// rename == a directory rename) apply here, and a user is expected to
// have far fewer Collections than Terms, so one small file read/written
// under a single lock is simpler and plenty fast at this scale.
type Collections struct {
	path string

	mu      sync.RWMutex
	entries map[string]*collectionEntry // key: normalized name
}

type collectionEntry struct {
	name  string
	tags  []string
	terms []string // order not meaningful, kept sorted for stable JSON diffs
}

// collectionFile is one Collection's on-disk shape, within the file-wide
// slice below.
type collectionFile struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Terms []string `json:"terms"`
}

var _ storage.CollectionStorage = (*Collections)(nil)

// NewCollections creates (if needed) root and loads collections.json
// already under it, if any.
func NewCollections(root string) (*Collections, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("diskgallery: create root %q: %w", root, err)
	}

	c := &Collections{path: filepath.Join(root, "collections.json"), entries: make(map[string]*collectionEntry)}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Collections) load() error {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return nil // no collections yet — fine, not an error
	}
	if err != nil {
		return fmt.Errorf("diskgallery: read %q: %w", c.path, err)
	}

	var files []collectionFile
	if err := json.Unmarshal(data, &files); err != nil {
		return fmt.Errorf("diskgallery: parse %q: %w", c.path, err)
	}
	for _, f := range files {
		c.entries[normalize(f.Name)] = &collectionEntry{name: normalize(f.Name), tags: f.Tags, terms: f.Terms}
	}
	return nil
}

// save rewrites the whole collections.json atomically (temp file +
// rename) — must be called with mu held. Cheap: a handful of small
// records, not the large-payload case Gallery's per-entry files exist
// for.
func (c *Collections) save() error {
	files := make([]collectionFile, 0, len(c.entries))
	for _, e := range c.entries {
		files = append(files, collectionFile{Name: e.name, Tags: e.tags, Terms: e.terms})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("diskgallery: marshal collections: %w", err)
	}

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("diskgallery: write collections: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("diskgallery: finalize collections: %w", err)
	}
	return nil
}

// Create — see storage.CollectionStorage.
func (c *Collections) Create(name string, tags []string) error {
	name = normalize(name)
	if name == "" {
		return fmt.Errorf("collection name cannot be empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[name]; exists {
		return fmt.Errorf("collection %q already exists", name)
	}
	c.entries[name] = &collectionEntry{name: name, tags: normalizeTags(tags), terms: nil}
	return c.save()
}

// Delete — see storage.CollectionStorage.
func (c *Collections) Delete(name string) {
	name = normalize(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[name]; !ok {
		return
	}
	delete(c.entries, name)
	_ = c.save()
}

// Rename — see storage.CollectionStorage.
func (c *Collections) Rename(oldName, newName string) error {
	oldName = normalize(oldName)
	newName = normalize(newName)
	if newName == "" {
		return fmt.Errorf("new collection name cannot be empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[oldName]
	if !ok {
		return fmt.Errorf("collection %q not found", oldName)
	}
	if _, exists := c.entries[newName]; exists {
		return fmt.Errorf("collection %q already exists", newName)
	}
	delete(c.entries, oldName)
	e.name = newName
	c.entries[newName] = e
	if err := c.save(); err != nil {
		// Roll back the in-memory rename too — keep the two in sync.
		delete(c.entries, newName)
		e.name = oldName
		c.entries[oldName] = e
		return err
	}
	return nil
}

// SetTags — see storage.CollectionStorage.
func (c *Collections) SetTags(name string, tags []string) error {
	name = normalize(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[name]
	if !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	e.tags = normalizeTags(tags)
	return c.save()
}

// AddTerm — see storage.CollectionStorage.
func (c *Collections) AddTerm(collectionName, termName string) error {
	collectionName = normalize(collectionName)
	termName = normalize(termName)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[collectionName]
	if !ok {
		return fmt.Errorf("collection %q not found", collectionName)
	}
	for _, t := range e.terms {
		if t == termName {
			return nil // already there — idempotent
		}
	}
	e.terms = append(e.terms, termName)
	sort.Strings(e.terms)
	return c.save()
}

// RemoveTerm — see storage.CollectionStorage.
func (c *Collections) RemoveTerm(collectionName, termName string) {
	collectionName = normalize(collectionName)
	termName = normalize(termName)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[collectionName]
	if !ok {
		return
	}
	for i, t := range e.terms {
		if t == termName {
			e.terms = append(e.terms[:i], e.terms[i+1:]...)
			_ = c.save()
			return
		}
	}
}

// Get — see storage.CollectionStorage.
func (c *Collections) Get(name string) (storage.Collection, bool) {
	name = normalize(name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[name]
	if !ok {
		return storage.Collection{}, false
	}
	return toCollection(e), true
}

// List — see storage.CollectionStorage.
func (c *Collections) List() []storage.Collection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]storage.Collection, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, toCollection(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func toCollection(e *collectionEntry) storage.Collection {
	tags := make([]string, len(e.tags))
	copy(tags, e.tags)
	terms := make([]string, len(e.terms))
	copy(terms, e.terms)
	return storage.Collection{Name: e.name, Tags: tags, Terms: terms}
}

// normalizeTags canonicalizes each tag (case/whitespace) and drops
// empties/duplicates, sorted — flat tags, combobox-managed
// ("réutiliser ou Créer", never free-typed duplicates with different
// casing, docs/gui/design-brief.md § Bibliothèque).
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = normalize(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
