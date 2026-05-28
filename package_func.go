package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseExpiry turns a token like "10M" into an absolute Unix expiry timestamp.
// The last character is the unit (S/M/H/D); everything before it is the count.
func parseExpiry(expiry string) (int64, error) {
	if len(expiry) < 2 {
		return 0, fmt.Errorf("invalid expiry: %q", expiry)
	}

	unit := string(expiry[len(expiry)-1])
	duration := expiry[:len(expiry)-1]
	seconds, err := strconv.Atoi(duration)
	if err != nil {
		return 0, err
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("invalid expiry: duration must be positive")
	}

	switch strings.ToUpper(unit) {
	case "S":
		return time.Now().Unix() + int64(seconds), nil
	case "M":
		return time.Now().Unix() + int64(seconds)*60, nil
	case "H":
		return time.Now().Unix() + int64(seconds)*60*60, nil
	case "D":
		return time.Now().Unix() + int64(seconds)*60*60*24, nil
	default:
		return 0, fmt.Errorf("invalid expiry unit")
	}
}

// setValue stores key=value. condition is one of:
//   - ""   : always set (unconditional)
//   - "NX" : only set if the key does NOT already exist
//   - "XX" : only set if the key DOES already exist
//
// These match Redis SET NX/XX semantics. A failed condition is reported as an
// error so the HTTP layer can translate it into a status code.
func (d *datastore) setValue(key, value string, expTime int64, condition string) error {
	// Reject an already-expired timestamp before taking the lock; this check
	// does not need the map and keeps the critical section small.
	if expTime != 0 && expTime < time.Now().Unix() {
		return fmt.Errorf("invalid expiry time")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	_, exists := d.data[key]
	switch condition {
	case "NX":
		if exists {
			return fmt.Errorf("key already exists: %s", key)
		}
	case "XX":
		if !exists {
			return fmt.Errorf("key does not exist: %s", key)
		}
	}

	// Replace the whole pointer rather than mutating fields in place. This is
	// what makes value/expTime safe to read later under only a read lock.
	d.data[key] = &dataValue{value: value, expTime: expTime}
	d.logger.Infof("set key=%s value=%s expTime=%d condition=%q", key, value, expTime, condition)
	return nil
}

// getValue returns the value for key, or an error if it is missing or expired.
// Expired keys are removed lazily (on access). The read happens under a read
// lock; the actual delete happens under a write lock, because deleting from a
// map while only holding RLock is a data race ("concurrent map read and map
// write" panic under the race detector / fatal at runtime).
func (d *datastore) getValue(key string) (string, error) {
	d.mu.RLock()
	v, ok := d.data[key]
	if !ok {
		d.mu.RUnlock()
		return "", fmt.Errorf("key not found")
	}
	value := v.value
	expired := v.expTime != 0 && v.expTime < time.Now().Unix()
	d.mu.RUnlock()

	if expired {
		d.deleteIfExpired(key)
		return "", fmt.Errorf("key not found")
	}
	return value, nil
}

// deleteIfExpired removes key under the write lock, re-checking expiry first so
// we never delete a key that another goroutine reset to a fresh value in the
// window between getValue releasing RLock and us acquiring Lock (TOCTOU guard).
func (d *datastore) deleteIfExpired(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.data[key]; ok && v.expTime != 0 && v.expTime < time.Now().Unix() {
		delete(d.data, key)
	}
}

// qPush appends values to the queue stored at key, creating it if needed.
// We take the map's write lock to find-or-create the entry, then guard the
// queue slice with that entry's own queueMu (lock order: mu -> queueMu).
func (d *datastore) qPush(key string, values ...string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dv, ok := d.data[key]
	if !ok {
		dv = &dataValue{}
		d.data[key] = dv
	}

	dv.queueMu.Lock()
	dv.queue = append(dv.queue, values...)
	dv.queueMu.Unlock()
	return nil
}

// qPop removes and returns the front element of the queue at key (FIFO).
// The bool is false when the key is missing or the queue is empty.
//
// Only a read lock is needed on the map (we are not changing which keys exist),
// and the queue mutation is guarded by the per-value queueMu. This is a plain
// synchronous function: the previous version spawned a goroutine that wrote to
// unbuffered channels and could block forever (goroutine + lock leak) when the
// caller did not drain both channels.
func (d *datastore) qPop(key string) (string, bool) {
	d.mu.RLock()
	dv, ok := d.data[key]
	d.mu.RUnlock()
	if !ok {
		return "", false
	}

	dv.queueMu.Lock()
	defer dv.queueMu.Unlock()
	if len(dv.queue) == 0 {
		return "", false
	}

	value := dv.queue[0]
	dv.queue = dv.queue[1:]
	return value, true
}

// getAll returns a snapshot of every live (non-expired) key. For queue keys the
// elements are joined with ", "; for plain keys the value is returned as-is.
func (d *datastore) getAll() map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now().Unix()
	result := make(map[string]string)
	for key, value := range d.data {
		if value.expTime != 0 && value.expTime < now {
			continue // skip expired keys instead of reporting stale data
		}
		value.queueMu.Lock()
		if len(value.queue) > 0 {
			result[key] = strings.Join(value.queue, ", ")
		} else {
			result[key] = value.value
		}
		value.queueMu.Unlock()
	}

	return result
}
