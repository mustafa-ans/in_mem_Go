package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	data := newDatastore(logger)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<h1>Hello from Go!</h1>")
	})

	// /set stores a key. Body: {"command": "<key> <value> [EX <n><unit>] [NX|XX]"}
	http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Command string `json:"command"` // struct tag maps JSON "command" -> Command
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		parts := strings.Fields(req.Command)
		if len(parts) < 2 {
			writeError(w, http.StatusBadRequest, "invalid command")
			return
		}

		key := parts[0]
		value := parts[1]
		var expTime int64
		var condition string // "", "NX", or "XX"

		for i := 2; i < len(parts); i++ {
			switch strings.ToUpper(parts[i]) {
			case "EX":
				if i+1 >= len(parts) {
					writeError(w, http.StatusBadRequest, "invalid command")
					return
				}
				exp, err := parseExpiry(parts[i+1])
				if err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				expTime = exp
				i++ // skip the value we just consumed
			case "NX":
				condition = "NX"
			case "XX":
				condition = "XX"
			default:
				writeError(w, http.StatusBadRequest, "invalid command")
				return
			}
		}

		if err := data.setValue(key, value, expTime, condition); err != nil {
			switch {
			case strings.HasPrefix(err.Error(), "key already exists"):
				writeError(w, http.StatusConflict, err.Error())
			case strings.HasPrefix(err.Error(), "key does not exist"):
				writeError(w, http.StatusNotFound, err.Error())
			case strings.HasPrefix(err.Error(), "invalid expiry"):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"message": "key set successfully"})
	})

	// /get?key=<key> returns the value for a key.
	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		if key == "" {
			writeError(w, http.StatusBadRequest, "key parameter not found in query string")
			return
		}

		value, err := data.getValue(key)
		if err != nil {
			if strings.HasPrefix(err.Error(), "key not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"value": value})
	})

	// /qpush appends to a queue. Body: {"command": "QPUSH", "args": ["<key>", "v1", "v2", ...]}
	http.HandleFunc("/qpush", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Cmd  string   `json:"command"`
			Args []string `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if req.Cmd != "QPUSH" {
			writeError(w, http.StatusBadRequest, "invalid command")
			return
		}
		if len(req.Args) < 2 {
			writeError(w, http.StatusBadRequest, "invalid command")
			return
		}

		key := req.Args[0]
		values := req.Args[1:]
		if err := data.qPush(key, values...); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"message": "values added to queue"})
	})

	// /qpop removes and returns the front of a queue. Body: {"command": "QPOP", "key": "<key>"}
	http.HandleFunc("/qpop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Cmd string `json:"command"`
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if req.Cmd != "QPOP" {
			writeError(w, http.StatusBadRequest, "invalid command")
			return
		}

		value, ok := data.qPop(req.Key)
		if !ok {
			writeError(w, http.StatusNotFound, "queue not found or empty")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"value": value})
	})

	// /getall returns every live (non-expired) key.
	http.HandleFunc("/getall", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		writeJSON(w, http.StatusOK, data.getAll())
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

// writeJSON marshals payload with encoding/json so all values are correctly
// escaped, sets the content type, and writes the status. This replaces the old
// hand-built `fmt.Fprintf(w, `{"value": "%s"}`, ...)` responses, which produced
// invalid JSON (and an injection vector) whenever a value contained a quote.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError is a small convenience wrapper for {"error": "..."} responses.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
