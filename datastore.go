package main

import "sync"

type datastore struct {
    data map[string]*dataValue
    mu   sync.RWMutex
    maxSizeBytes int // Maximum size
    sizeBytes    int // Current size
}

type dataValue struct {
    value    string
    expTime  int64 // Expiry time in Unix timestamp format
    isExists bool  // Used to check existence of key for conditional set operation
    queueMu sync.Mutex // Mutex for the queue field
    queue   []string
}
