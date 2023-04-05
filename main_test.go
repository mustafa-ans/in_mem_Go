package main_test

import (
    "fmt"
    "time"
)

func BenchmarkQPop(qpop func(key string, valChan chan string, okChan chan bool)) {
    // initialize test data
    data := &datastore{
        data: map[string]*dataValue{
            "test1": &dataValue{
                value: "value1",
            },
            "test2": &dataValue{
                value: "value2",
                queue: []string{"value3", "value4", "value5"},
            },
        },
    }

    // start the benchmark
    numOps := 100000
    start := time.Now()
    for i := 0; i < numOps; i++ {
        key := fmt.Sprintf("test%d", i%2+1)
        valChan := make(chan string)
        okChan := make(chan bool)
        go qpop(key, valChan, okChan)
        <-valChan
        <-okChan
    }
    elapsed := time.Since(start)

    // print benchmark results
    fmt.Printf("BenchmarkQPop: %d operations in %s, %f ops/sec\n", numOps, elapsed, float64(numOps)/elapsed.Seconds())
}
