package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// test

func main() {
	data := &datastore{
		data:         make(map[string]*dataValue),
		maxSizeBytes: 256 * 1024,
		logger:       logrus.New()}

	data.logger.SetOutput(os.Stdout)

	// handler for /
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<h1>Hello from Go!</h1>")
	})

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

	//qpop
	http.HandleFunc("/qpop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		type Command struct {
			Cmd string `json:"command"`
			Key string `json:"key"`
		}

		var req Command
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "%s"}`, err.Error())
			return
		}

		if req.Cmd != "QPOP" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "invalid command"}`)
			return
		}

		valChan := make(chan string)
		okChan := make(chan bool)
		go data.qPop(req.Key, valChan, okChan)

		select {
		case value := <-valChan:
			if value == "" {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error": "queue not found or empty"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			if ok := <-okChan; !ok {
				fmt.Fprint(w, `{"message": "queue is empty"}`)
				return
			}

			fmt.Fprintf(w, `{"value": "%s"}`, value)
		case ok := <-okChan:
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error": "queue not found or empty"}`)
				return
			}
		}
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
