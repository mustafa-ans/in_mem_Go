package main

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type datastore struct {
	data         map[string]*dataValue
	mu           sync.RWMutex
	maxSizeBytes int // Maximum size
	logger       *logrus.Logger
	// sizeBytes    int // Current size
	// lruCacheHead *LRUCacheNode
	// lruCacheTail *LRUCacheNode
	// lruCacheMap  map[string]*LRUCacheNode
}

type dataValue struct {
	value    string
	expTime  int64      // Expiry time in Unix timestamp format
	isExists bool       // Used to check existence of key for conditional set operation
	queueMu  sync.Mutex // Mutex for the queue field
	queue    []string
}

type LRUCacheNode struct {
	// key   string
	// value string
	// prev  *LRUCacheNode
	// next  *LRUCacheNode
}
