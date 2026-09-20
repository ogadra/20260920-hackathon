package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type recordedRequest struct {
	Method string              `json:"method"`
	Path   string              `json:"path"`
	Host   string              `json:"host"`
	Header map[string][]string `json:"header"`
	Body   string              `json:"body"`
}

type recorder struct {
	mu     sync.Mutex
	req    *recordedRequest
	status int
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic(name + " environment variable is required")
	}
	return value
}

func (r *recorder) set(req *http.Request, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.req = &recordedRequest{
		Method: req.Method,
		Path:   req.URL.RequestURI(),
		Host:   req.Host,
		Header: req.Header.Clone(),
		Body:   body,
	}
}

func (r *recorder) reset(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	r.req = nil
	r.status = http.StatusNoContent
	r.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (r *recorder) setStatus(w http.ResponseWriter, req *http.Request) {
	code := req.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	status, err := strconv.Atoi(code)
	if err != nil || status < 100 || status > 599 {
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (r *recorder) responseStatus() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *recorder) last(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	req := r.req
	r.mu.Unlock()
	if req == nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(req); err != nil {
		log.Printf("encode last request: %v", err)
	}
}

func main() {
	rec := &recorder{status: http.StatusNoContent}

	control := http.NewServeMux()
	control.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	control.HandleFunc("/__reset", rec.reset)
	control.HandleFunc("/__status", rec.setStatus)
	control.HandleFunc("/__last", rec.last)
	go func() {
		if err := http.ListenAndServe(":8080", control); err != nil {
			log.Fatalf("control server: %v", err)
		}
	}()

	certFile := requireEnv("FORWARD_TARGET_CERT_FILE")
	keyFile := requireEnv("FORWARD_TARGET_KEY_FILE")
	target := http.NewServeMux()
	target.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		rec.set(req, string(body))
		w.WriteHeader(rec.responseStatus())
	})
	srv := &http.Server{
		Addr:    ":443",
		Handler: target,
	}
	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil {
		log.Fatalf("https server: %v", err)
	}
}
