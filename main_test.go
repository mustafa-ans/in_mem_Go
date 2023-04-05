package main

import (
    "strconv"
    "testing"
    "time"
)

func TestGetValue(t *testing.T) {
    data := &datastore{data: make(map[string]*dataValue)}

    // Test for getting an existing key
    key := "test-key"
    value := "test-value"
    expTime := time.Now().Add(time.Hour).Unix()
    data.setValue(key, value, expTime, false)
    res, err := data.getValue(key)
    if err != nil {
        t.Errorf("getValue(%q) failed: %s", key, err)
    }
    if res != value {
        t.Errorf("getValue(%q) = %q, want %q", key, res, value)
    }

    // Test for getting an expired key
    key = "expired-key"
    value = "expired-value"
    expTime = time.Now().Add(-time.Hour).Unix()
    data.setValue(key, value, expTime, false)
    _, err = data.getValue(key)
    if err == nil {
        t.Errorf("getValue(%q) did not return an error for an expired key", key)
    }

    // Test for getting a non-existent key
    key = "non-existent-key"
    _, err = data.getValue(key)
    if err == nil {
        t.Errorf("getValue(%q) did not return an error for a non-existent key", key)
    }

}

func benchmarkQPush(b *testing.B, numValues int) {
    data := &datastore{data: make(map[string]*dataValue)}
    key := "testkey"

    // Generate values
    values := make([]string, numValues)
    for i := 0; i < numValues; i++ {
        values[i] = strconv.Itoa(i)
    }

    // Run benchmark
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        err := data.qPush(key, values...)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkQPush10(b *testing.B) {
    benchmarkQPush(b, 10)
}

func BenchmarkQPush100(b *testing.B) {
    benchmarkQPush(b, 100)
}

func BenchmarkQPush1000(b *testing.B) {
    benchmarkQPush(b, 1000)
}