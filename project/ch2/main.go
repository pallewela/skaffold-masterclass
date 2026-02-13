// main.go — Hello App: Chapter 2 — Your first Go service on Kubernetes.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Response is the JSON payload returned by our API.
type Response struct {
	Message   string `json:"message"`
	Hostname  string `json:"hostname"`
	Timestamp string `json:"timestamp"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)

	log.Printf("🚀 Server starting on port %s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}

// handleRoot returns a JSON greeting with the pod's hostname.
// In Kubernetes, the hostname is the Pod name — useful for
// verifying which Pod is serving your request.
func handleRoot(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()

	resp := Response{
		Message:   "Hello from Skaffold! 🎉",
		Hostname:  hostname,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("✅ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}

// handleHealth is a simple liveness endpoint.
// Kubernetes will use this to check if your Pod is alive.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
