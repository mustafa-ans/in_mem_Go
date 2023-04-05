package main

import (
    "strconv"
    "testing"
)

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
