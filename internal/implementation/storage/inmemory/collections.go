package inmemory

import (
	"fmt"
	"sort"
	"sync"

	"live-semantic/internal/infrastructure/storage"
)

// Collections implements storage.CollectionStorage, process-RAM only —
// same pairing as Gallery/diskgallery.Gallery: this is the lightweight
// adapter for tests/tools, implementation/storage/diskgallery.Collections
// is the persisted one main.go actually wires in.
type Collections struct {
	mu      sync.RWMutex
	entries map[string]*collectionEntry // key: normalized name
}

type collectionEntry struct {
	name  string
	tags  []string
	terms map[string]struct{} // set, order doesn't matter — List sorts
}

var _ storage.CollectionStorage = (*Collections)(nil)

// NewCollections creates an empty Collections store.
func NewCollections() *Collections {
	return &Collections{entries: make(map[string]*collectionEntry)}
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
	c.entries[name] = &collectionEntry{name: name, tags: normalizeTags(tags), terms: make(map[string]struct{})}
	return nil
}

// Delete — see storage.CollectionStorage.
func (c *Collections) Delete(name string) {
	name = normalize(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, name)
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
	return nil
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
	e.terms[termName] = struct{}{}
	return nil
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
	delete(e.terms, termName)
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
	terms := make([]string, 0, len(e.terms))
	for t := range e.terms {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	tags := make([]string, len(e.tags))
	copy(tags, e.tags)
	return storage.Collection{Name: e.name, Tags: tags, Terms: terms}
}

// normalizeTags canonicalizes each tag (case/whitespace, same rule as
// normalize) and drops empties/duplicates — flat tags, combobox-managed,
// "football" and "Football " must collapse to the same tag (§
// Bibliothèque, docs/gui/design-brief.md).
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
