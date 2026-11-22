package store

import "Cryon2/internal/domain"

// Logical counters avoid clock skew. Origin breaks concurrent ties deterministically.
type LibraryEntry struct {
	Track   domain.Track `json:"track"`
	Counter int64        `json:"counter"`
	Origin  string       `json:"origin"`
	Deleted bool         `json:"deleted"`
}
