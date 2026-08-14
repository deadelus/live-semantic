// Package inmemory is the only journal.Journal adapter today — a bounded
// in-process ring buffer, no persistence across restarts. Same "no
// vectorial/real database at this scale" position already stated
// elsewhere in this codebase (implementation/storage/inmemory's own doc
// comment) applies here too: a few hundred recent events is plenty for a
// local-dev/single-operator tool, and unlike the reference gallery
// (implementation/storage/diskgallery), losing the journal on restart
// isn't a real loss — it's an operational log, not user-curated data.
package inmemory

import (
	"sort"
	"sync"

	"live-semantic/internal/infrastructure/journal"
)

// maxEntries bounds memory the same way
// implementation/streamer/output.RingBufferOutput's maxEntries does —
// independent of any age-based eviction (there is none here, a journal
// entry doesn't go stale the way a rewind frame does), just a hard cap.
const maxEntries = 500

// Journal implements journal.Journal as a fixed-capacity ring buffer.
type Journal struct {
	mu      sync.Mutex
	entries []journal.Entry
	next    int // write cursor once entries is at capacity
	full    bool
}

var _ journal.Journal = (*Journal)(nil)

// New creates an empty Journal.
func New() *Journal {
	return &Journal{entries: make([]journal.Entry, 0, maxEntries)}
}

// Record — see journal.Journal.
func (j *Journal) Record(entry journal.Entry) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.entries) < maxEntries {
		j.entries = append(j.entries, entry)
		return
	}
	j.entries[j.next] = entry
	j.next = (j.next + 1) % maxEntries
	j.full = true
}

// List — see journal.Journal. Newest first — a journal drawer reads
// top-to-bottom as "most recent event first", same convention as most
// log viewers.
func (j *Journal) List() []journal.Entry {
	j.mu.Lock()
	defer j.mu.Unlock()

	out := make([]journal.Entry, len(j.entries))
	copy(out, j.entries)
	sort.Slice(out, func(i, k int) bool { return out[i].Timestamp.After(out[k].Timestamp) })
	return out
}
