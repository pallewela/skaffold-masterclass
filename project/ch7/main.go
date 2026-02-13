// main.go — Hello App: Chapter 7 — Profiles & Multi-Config.
// Same server as Ch6. Focus is on skaffold.yaml profiles.
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
	Env       string `json:"env,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)

	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" {
		log.Printf("🌍 Running in environment: %s", appEnv)
	}

	log.Printf("🚀 Server starting on port %s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()

	resp := Response{
		Message:   "Hello from Skaffold! 🎉",
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
