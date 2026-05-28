package main

import (
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// newTestStore builds a store whose logs are discarded, so tests stay quiet.
func newTestStore() *datastore {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return newDatastore(logger)
}

func TestSetAndGet(t *testing.T) {
	d := newTestStore()

	if err := d.setValue("k", "v", 0, ""); err != nil {
		t.Fatalf("setValue: %v", err)
	}
	got, err := d.getValue("k")
	if err != nil {
		t.Fatalf("getValue: %v", err)
	}
	if got != "v" {
		t.Errorf("getValue = %q, want %q", got, "v")
	}
}

func TestGetMissingKey(t *testing.T) {
	d := newTestStore()
	if _, err := d.getValue("nope"); err == nil {
		t.Error("expected error for missing key, got nil")
	}
}

// TestExpiredKeyIsLazilyDeleted exercises the lazy-expiration path. We insert
// directly into the map with a past expiry, because setValue deliberately
// rejects past expiries and so cannot create an already-expired entry.
func TestExpiredKeyIsLazilyDeleted(t *testing.T) {
	d := newTestStore()
	d.data["old"] = &dataValue{value: "v", expTime: time.Now().Add(-time.Hour).Unix()}

	if _, err := d.getValue("old"); err == nil {
		t.Error("expected error for expired key, got nil")
	}

	d.mu.RLock()
	_, stillThere := d.data["old"]
	d.mu.RUnlock()
	if stillThere {
		t.Error("expired key was not deleted on access")
	}
}

func TestSetNX(t *testing.T) {
	d := newTestStore()

	if err := d.setValue("k", "v1", 0, "NX"); err != nil {
		t.Fatalf("NX on a new key should succeed: %v", err)
	}
	if err := d.setValue("k", "v2", 0, "NX"); err == nil {
		t.Error("NX on an existing key should fail")
	}
	if got, _ := d.getValue("k"); got != "v1" {
		t.Errorf("value should be unchanged after failed NX, got %q", got)
	}
}

func TestSetXX(t *testing.T) {
	d := newTestStore()

	if err := d.setValue("k", "v1", 0, "XX"); err == nil {
		t.Error("XX on a missing key should fail")
	}
	if err := d.setValue("k", "v1", 0, ""); err != nil {
		t.Fatalf("unconditional set: %v", err)
	}
	if err := d.setValue("k", "v2", 0, "XX"); err != nil {
		t.Fatalf("XX on an existing key should succeed: %v", err)
	}
	if got, _ := d.getValue("k"); got != "v2" {
		t.Errorf("XX should have updated the value, got %q", got)
	}
}

func TestQueueFIFO(t *testing.T) {
	d := newTestStore()

	if err := d.qPush("q", "a", "b", "c"); err != nil {
		t.Fatalf("qPush: %v", err)
	}
	for _, want := range []string{"a", "b", "c"} {
		got, ok := d.qPop("q")
		if !ok {
			t.Fatalf("qPop returned ok=false, want %q", want)
		}
		if got != want {
			t.Errorf("qPop = %q, want %q", got, want)
		}
	}
	if _, ok := d.qPop("q"); ok {
		t.Error("qPop on a drained queue should return ok=false")
	}
}

func TestQPopMissingQueue(t *testing.T) {
	d := newTestStore()
	if _, ok := d.qPop("missing"); ok {
		t.Error("qPop on a missing queue should return ok=false")
	}
}

func TestGetAllSkipsExpired(t *testing.T) {
	d := newTestStore()

	if err := d.setValue("live", "v", 0, ""); err != nil {
		t.Fatalf("setValue: %v", err)
	}
	d.data["dead"] = &dataValue{value: "x", expTime: time.Now().Add(-time.Hour).Unix()}

	all := d.getAll()
	if _, ok := all["live"]; !ok {
		t.Error("getAll should include the live key")
	}
	if _, ok := all["dead"]; ok {
		t.Error("getAll should skip the expired key")
	}
}

// TestConcurrentAccess hammers the store from many goroutines on overlapping
// keys. Its real job is to be run under the race detector: `go test -race`.
// If the locking is wrong, the detector reports a data race here.
func TestConcurrentAccess(t *testing.T) {
	d := newTestStore()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			key := "k" + strconv.Itoa(n%5)       // overlap keys on purpose
			queue := "queue" + strconv.Itoa(n%5)  // overlap queues on purpose
			_ = d.setValue(key, strconv.Itoa(n), 0, "")
			_, _ = d.getValue(key)
			_ = d.qPush(queue, strconv.Itoa(n))
			_, _ = d.qPop(queue)
			_ = d.getAll()
		}(i)
	}
	wg.Wait()
}

func benchmarkQPush(b *testing.B, numValues int) {
	d := newTestStore()
	key := "testkey"

	values := make([]string, numValues)
	for i := 0; i < numValues; i++ {
		values[i] = strconv.Itoa(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := d.qPush(key, values...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQPush10(b *testing.B)   { benchmarkQPush(b, 10) }
func BenchmarkQPush100(b *testing.B)  { benchmarkQPush(b, 100) }
func BenchmarkQPush1000(b *testing.B) { benchmarkQPush(b, 1000) }
