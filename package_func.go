package main

import (
	"fmt"
	"go/ast"
    "go/token"
    "go/parser"
	"time"
	"strings"
	"strconv"

)


func parseExpiry(expiry string) (int64, error) {
    unit := string(expiry[len(expiry)-1])
    duration := expiry[:len(expiry)-1]
    seconds, err := strconv.Atoi(duration)
    if err != nil {
        return 0, err
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



func (d *datastore) setValue(key, value string, expTime int64, isExists bool) error {
    // Tokenize the input values
    fset := token.NewFileSet()
    expr, err := parser.ParseExpr(fmt.Sprintf("%q", key+" "+value))
    if err != nil {
        return err
    }
    ast.Inspect(expr, func(n ast.Node) bool {
        if n != nil {
            // Remove the computation of f if it is not needed
            fmt.Printf("Token: %s, Position: %v\n", fset.Position(n.Pos()), n)
        }
        return true
    })

    d.mu.Lock()
    defer d.mu.Unlock()

    if _, ok := d.data[key]; ok && isExists {
        return fmt.Errorf("key already exists: %s", key)
    }

    if expTime != 0 && expTime < time.Now().Unix() {
        // If expiry time is in past, skip setting the value
        return fmt.Errorf("invalid expiry time")
    }

    d.data[key] = &dataValue{value: value, expTime: expTime, isExists: true}
    return nil
}

func (d *datastore) getValue(key string) (string, error) {
    d.mu.RLock()
    defer d.mu.RUnlock()

    if v, ok := d.data[key]; ok {
        if v.expTime != 0 && v.expTime < time.Now().Unix() {
            // If expiry time is in past, delete the key
            delete(d.data, key)
            return "", fmt.Errorf("key not found")
        }
        return v.value, nil
    }
    return "", fmt.Errorf("key not found")
}

func (d *datastore) qPush(key string, values ...string) error {
    d.mu.Lock()
    defer d.mu.Unlock()

    if _, ok := d.data[key]; !ok {
        d.data[key] = &dataValue{}
    }

    if d.data[key].queue == nil {
        d.data[key].queue = make([]string, 0)
    }

    d.data[key].queue = append(d.data[key].queue, values...)

    return nil
}

func (d *datastore) qPop(key string, valChan chan string, okChan chan bool) {
    d.mu.RLock()
    defer d.mu.RUnlock()

    if _, ok := d.data[key]; !ok {
        valChan <- ""
        okChan <- false
        return
    }

    d.data[key].queueMu.Lock()
    defer d.data[key].queueMu.Unlock()

    if len(d.data[key].queue) == 0 {
        valChan <- ""
        okChan <- false
        return
    }

    value := d.data[key].queue[0]
    d.data[key].queue = d.data[key].queue[1:]

    valChan <- value
    okChan <- true
}

// getall
func (d *datastore) getAll() map[string]string {
    d.mu.RLock()
    defer d.mu.RUnlock()

    result := make(map[string]string)
    for key, value := range d.data {
        if len(value.queue) > 0 {
			result[key] = strings.Join(value.queue, ", ")
		} else {
			result[key] = value.value
		}
    }
    return result
}