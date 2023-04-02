package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "sync"
    "time"
	"strconv"
)

type datastore struct {
    data map[string]*dataValue
    mu   sync.RWMutex
}

type dataValue struct {
    value    string
    expTime  int64 // Expiry time in Unix timestamp format
    isExists bool  // Used to check existence of key for conditional set operations
	queue    []string
}
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

func main() {
    data := &datastore{data: make(map[string]*dataValue)}

    http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        var req struct {
            Command string `json:"command"` //struct tag
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            w.WriteHeader(http.StatusBadRequest)
            fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
            return
        }

        parts := strings.Fields(req.Command)
        if len(parts) < 2 {
            w.WriteHeader(http.StatusBadRequest)
            fmt.Fprint(w, `{"error": "invalid command"}`)
            return
        }

        key := parts[0]
        value := parts[1]
        var expTime int64
        var isExists bool

        for i := 2; i < len(parts); i++ {
            if parts[i] == "EX" {
                if i+1 >= len(parts) {
                    w.WriteHeader(http.StatusBadRequest)
                    fmt.Fprint(w, `{"error": "invalid command"}`)
                    return
                }
                var err error
                expTime, err = parseExpiry(parts[i+1])
                if err != nil {
                    w.WriteHeader(http.StatusBadRequest)
                    fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
                    return
                }
                i++
            } else if parts[i] == "NX" {
                isExists = false
            } else if parts[i] == "XX" {
                isExists = true
            } else {
                w.WriteHeader(http.StatusBadRequest)
                fmt.Fprint(w, `{"error": "invalid command"}`)
                return
            }
        }

        err := data.setValue(key, value, expTime, isExists)
        if err != nil {
            if strings.HasPrefix(err.Error(), "key already exists") {
                w.WriteHeader(http.StatusConflict)
            } else if strings.HasPrefix(err.Error(), "invalid expiry time") {
                w.WriteHeader(http.StatusBadRequest)
            } else {
                w.WriteHeader(http.StatusInternalServerError)
            }
            fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
            return
        }

        w.WriteHeader(http.StatusCreated)
        fmt.Fprintf(w, `{"message": "key set successfully"}`)
    })
	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "GET" {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        key := r.URL.Query().Get("key")
        if key == "" {
            w.WriteHeader(http.StatusBadRequest)
            fmt.Fprint(w, `{"error": "key parameter not found in query string"}`)
            return
        }

        value, err := data.getValue(key)
        if err != nil {
            if strings.HasPrefix(err.Error(), "key not found") {
                w.WriteHeader(http.StatusNotFound)
            } else {
                w.WriteHeader(http.StatusInternalServerError)
            }
            fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
            return
        }

        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{"value": "%s"}`, value)
    })

	http.HandleFunc("/qpush", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		
		type Command struct {
			Cmd  string   `json:"command"`
			Args []string `json:"args"`
		}
		
		var req Command
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
			return
		}
	
		if req.Cmd != "QPUSH" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "invalid command"}`)
			return
		}
	
		if len(req.Args) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "invalid command"}`)
			return
		}
	
		key := req.Args[0]
		values := req.Args[1:]
	
		if err := data.qPush(key, values...); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
			return
		}
	
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"message": "values added to queue"}`)
	})
	

	// get all
	http.HandleFunc("/getall", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	
		allData := data.getAll()
		jsonData, err := json.Marshal(allData)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
			return
		}
	
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
	

    if err := http.ListenAndServe(":8080", nil); err != nil {
        panic(err)
    }
}
