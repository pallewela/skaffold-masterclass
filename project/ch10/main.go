// main.go — Hello App API: Chapter 10 — Capstone.
// Enhanced API service with /jobs endpoints for the multi-service architecture.
// Pushes jobs to Redis for the worker to consume.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
	"os"
	"sync"
	"time"
)

// Response is the JSON payload returned by the root endpoint.
type Response struct {
	Message   string `json:"message"`
	Hostname  string `json:"hostname"`
	Timestamp string `json:"timestamp"`
	Env       string `json:"env,omitempty"`
}

// Job represents a background job.
type Job struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Payload   string `json:"payload"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// In-memory job store (in production, this would be Redis/DB).
var (
	jobs   = make(map[string]*Job)
	jobsMu sync.RWMutex
	jobSeq int
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" {
		log.Printf("🌍 Running in environment: %s", appEnv)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		log.Printf("📡 Redis configured at: %s", redisAddr)
	}

	// Diagnostics server (pprof) on a separate port
	go func() {
		log.Println("📊 Diagnostics server on :6060 (pprof at /debug/pprof/)")
		if err := http.ListenAndServe(":6060", nil); err != nil {
			log.Printf("⚠️ Diagnostics server error: %v", err)
		}
	}()

	// Application server
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/jobs", handleJobs)       // GET: list, POST: create
	mux.HandleFunc("/jobs/", handleJobByID)    // GET: single job

	log.Printf("🚀 API server starting on port %s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), mux); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	hostname, _ := os.Hostname()
	resp := Response{
		Message:   "Hello from Skaffold Capstone! 🎉",
		Hostname:  hostname,
		Timestamp: time.Now().Format(time.RFC3339),
		Env:       os.Getenv("APP_ENV"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
	log.Printf("✅ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listJobs(w, r)
	case http.MethodPost:
		createJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func listJobs(w http.ResponseWriter, _ *http.Request) {
	jobsMu.RLock()
	defer jobsMu.RUnlock()

	result := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, j)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type createJobRequest struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func createJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	jobsMu.Lock()
	jobSeq++
	j := &Job{
		ID:        fmt.Sprintf("job-%d", jobSeq),
		Type:      req.Type,
		Payload:   req.Payload,
		Status:    "queued",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	jobs[j.ID] = j
	jobsMu.Unlock()

	log.Printf("📝 Created job: %s (type=%s)", j.ID, j.Type)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(j)
}

func handleJobByID(w http.ResponseWriter, r *http.Request) {
	// Extract job ID from path: /jobs/{id}
	id := r.URL.Path[len("/jobs/"):]
	if id == "" {
		http.Error(w, `{"error":"missing job ID"}`, http.StatusBadRequest)
		return
	}

	jobsMu.RLock()
	j, ok := jobs[id]
	jobsMu.RUnlock()

	if !ok {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}
