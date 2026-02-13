// worker/main.go — Background Worker: Chapter 10 — Capstone.
// Processes jobs from a queue. In production this would consume from Redis;
// for the tutorial it runs a polling loop and logs its activity.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" {
		log.Printf("🌍 Worker running in environment: %s", appEnv)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		log.Printf("📡 Worker connecting to Redis at: %s", redisAddr)
	}

	// Health endpoint for the worker (liveness/readiness probes)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok","role":"worker"}`))
		})
		log.Println("🏥 Worker health endpoint on :8081")
		if err := http.ListenAndServe(":8081", mux); err != nil {
			log.Printf("⚠️ Worker health server error: %v", err)
		}
	}()

	log.Println("⚙️ Worker started — polling for jobs...")

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			processJobs()
		case sig := <-stop:
			log.Printf("🛑 Worker received %s — shutting down gracefully", sig)
			return
		}
	}
}

func processJobs() {
	// In production: BRPOP from Redis queue, process each job.
	// For the tutorial, we log a heartbeat.
	log.Println(fmt.Sprintf("💓 Worker heartbeat at %s — no jobs in queue",
		time.Now().Format(time.RFC3339)))
}
