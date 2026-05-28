package main

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// datastore is the top-level, in-memory key-value store.
//
// Concurrency model (read this before touching the locking code):
//   - mu guards the data map itself: which keys exist and which *dataValue
//     pointer each key maps to.
//   - each dataValue has its own queueMu that guards that value's queue slice.
//   - value and expTime are set once, when the *dataValue is created, and are
//     never mutated in place. Because they are effectively immutable, they can
//     be read under mu.RLock() without any further locking.
//   - Lock ordering is ALWAYS mu -> queueMu, never the reverse. Following a
//     single global ordering is what guarantees we can never deadlock.
type datastore struct {
	data   map[string]*dataValue
	mu     sync.RWMutex
	logger *logrus.Logger
}

// dataValue is the stored entry for a single key. A key is used either as a
// plain value (value/expTime) or as a queue (queue) depending on which command
// created it; see the "known limitations" note in the guide about mixing both.
type dataValue struct {
	value   string     // immutable after creation
	expTime int64      // Unix expiry timestamp; 0 means "never expires" (immutable)
	queueMu sync.Mutex // guards queue
	queue   []string
}

// newDatastore builds a ready-to-use store. Constructing through this function
// (rather than a bare struct literal) guarantees the map and logger are always
// initialised, so callers never hit a nil map or nil logger at runtime.
func newDatastore(logger *logrus.Logger) *datastore {
	return &datastore{
		data:   make(map[string]*dataValue),
		logger: logger,
	}
}
