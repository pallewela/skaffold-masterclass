// main.go — Hello App: Chapter 9 — CI/CD.
// Same as Ch8 — the new features are CI pipelines + verify stanza.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers on DefaultServeMux
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

	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" {
		log.Printf("🌍 Running in environment: %s", appEnv)
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

	log.Printf("🚀 Server starting on port %s", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), mux); err != nil {
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
