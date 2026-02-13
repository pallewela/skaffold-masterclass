# Chapter 8 — Port Forwarding, Logging, Lifecycle Hooks & Custom Actions

> *"The best developer tools are the ones you forget are running."*

---

## What You'll Learn

- **Port forwarding** — exposing Kubernetes services to `localhost` without `kubectl port-forward`
- **Log tailing** — streaming structured logs from multiple pods in one terminal
- **Lifecycle hooks** — running commands before/after build and deploy stages
- **Custom actions** — running database migrations, seed scripts, and other workflows
- **Health checks** — configuring `statusCheck` for reliable deploy feedback

---

## 8.1 — Port Forwarding Deep-Dive

We've been using `portForward` since Chapter 2, but there's much more to it than a single service mapping.

### Basic Configuration

```yaml
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080
```

This tells Skaffold: *"Forward traffic from `localhost:8080` to the Kubernetes Service `hello-app` on port `8080`."*

### Multiple Port Forwards

Real projects need more than one:

```yaml
portForward:
  # Application
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080

  # Metrics / pprof
  - resourceType: service
    resourceName: hello-app
    port: 6060
    localPort: 6060

  # Database (if deployed in-cluster)
  - resourceType: service
    resourceName: postgres
    namespace: default
    port: 5432
    localPort: 5432

  # Redis
  - resourceType: service
    resourceName: redis
    port: 6379
    localPort: 6379
```

### Resource Types

You can forward to different Kubernetes resource types:

| `resourceType` | Forwards To | Use Case |
|-----------------|-------------|----------|
| `service` | A Kubernetes Service | Most common — load-balanced across pods |
| `deployment` | A Deployment's pods | When there's no Service defined |
| `pod` | A specific Pod | Debugging a specific instance |

### Address Binding

By default, Skaffold binds to `127.0.0.1` (localhost only). To expose on all interfaces (e.g., for testing from another device):

```yaml
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080
    address: 0.0.0.0        # ⚠️ Exposes to your local network
```

> **Security warning:** Only use `0.0.0.0` on trusted networks. On a coffee-shop WiFi, this exposes your dev service to everyone on the network.

### Automatic Port Forwarding

Skaffold can auto-forward **all** Services and containerPorts without explicit configuration:

```bash
skaffold dev --port-forward
```

This discovers every Service and container port in your deployed manifests and forwards them automatically. Port numbers are assigned as:

1. If the `containerPort` is available locally → use the same port
2. If it's taken → increment until a free port is found

You'll see:

```
Port forwarding service/hello-app in namespace default,
  remote port 8080 -> http://127.0.0.1:8080
Port forwarding service/postgres in namespace default,
  remote port 5432 -> http://127.0.0.1:5432
```

---

## 8.2 — Adding a Metrics Endpoint to Our Project

Let's enhance our Go service with a `/debug/pprof` endpoint and a `/metrics` endpoint, then forward both ports.

### Update `main.go`

Add a separate HTTP server for diagnostics on port `6060`:

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    _ "net/http/pprof"   // Register pprof handlers on DefaultServeMux
    "os"
    "time"
)

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

    // Diagnostics server (pprof) on a separate port
    go func() {
        log.Println("📊 Diagnostics server on :6060")
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
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
    log.Printf("✅ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"ok"}`))
}
```

### Update `skaffold.yaml` Port Forwards

```yaml
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080
  - resourceType: deployment
    resourceName: hello-app
    port: 6060
    localPort: 6060
```

### Verify

```bash
skaffold dev

# In another terminal:
curl http://localhost:8080/          # Application
curl http://localhost:6060/debug/pprof/  # pprof index
```

---

## 8.3 — Log Tailing

### Default Behaviour

When you run `skaffold dev`, Skaffold automatically tails logs from all deployed containers. Each line is prefixed with the container name:

```
[hello-app] 🚀 Server starting on port 8080
[hello-app] ✅ GET / from 10.244.0.1:54321
```

### Structured Logging in Go

For production-grade logging, use structured output. Here's a minimal approach with the standard `log/slog` package (Go 1.21+):

```go
import "log/slog"

func main() {
    // JSON-formatted structured logs
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))
    slog.SetDefault(logger)

    slog.Info("server starting",
        "port", port,
        "env", os.Getenv("APP_ENV"),
    )
}
```

Output:

```json
{"time":"2026-02-13T12:30:00Z","level":"INFO","msg":"server starting","port":"8080","env":"development"}
```

### Filtering Logs

Skaffold doesn't provide built-in log filtering, but you can pipe its output:

```bash
# Only show lines containing "ERROR"
skaffold dev 2>&1 | grep "ERROR"

# Pretty-print JSON logs with jq
skaffold dev 2>&1 | grep -o '{.*}' | jq .

# Use stern for richer multi-pod log tailing
stern hello-app --output json | jq .
```

### Disabling Log Tailing

If logs are too noisy during development:

```bash
skaffold dev --tail=false
```

Then manually tail when you need to:

```bash
kubectl logs -l app=hello-app -f
```

---

## 8.4 — Lifecycle Hooks

Lifecycle hooks let you run custom commands at specific points in the Skaffold pipeline:

```
 before-build    build    after-build    before-deploy    deploy    after-deploy
      │             │           │              │             │            │
      ▼             ▼           ▼              ▼             ▼            ▼
 ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌──────────┐  ┌─────────┐  ┌─────────┐
 │ gen code │  │ docker  │  │ run     │  │ migrate  │  │ kubectl │  │ seed    │
 │ lint     │  │ build   │  │ tests   │  │ database │  │ apply   │  │ data    │
 └─────────┘  └─────────┘  └─────────┘  └──────────┘  └─────────┘  └─────────┘
```

### Hook Syntax

```yaml
build:
  hooks:
    before:
      - command: ["sh", "-c", "echo 'Starting build...'"]
    after:
      - command: ["sh", "-c", "echo 'Build complete!'"]

deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      before:
        - host:
            command: ["sh", "-c", "echo 'About to deploy...'"]
      after:
        - host:
            command: ["sh", "-c", "echo 'Deploy done!'"]
```

### Host vs. Container Hooks

| Hook Type | Runs Where | Use Case |
|-----------|-----------|----------|
| `host` | Your local machine | Code generation, running tests, migrations via CLI |
| `container` | Inside a running pod | Warm up caches, run in-cluster health checks |

### Container Hook Example

Run a command inside a container after deployment:

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      after:
        - container:
            command: ["sh", "-c", "echo 'Container is live!'"]
            containerName: hello-app
            podName: hello-app-*
```

---

## 8.5 — Real-World Hook Patterns

### Pattern 1: Code Generation Before Build

If your Go project uses `go generate` (e.g., for protobuf, mocks, or embedded assets):

```yaml
build:
  hooks:
    before:
      - command: ["go", "generate", "./..."]
  artifacts:
    - image: hello-app
      docker:
        dockerfile: Dockerfile
```

### Pattern 2: Database Migration Before Deploy

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      before:
        - host:
            command: ["sh", "-c", "./scripts/migrate.sh"]
            os: [linux, darwin]
```

`scripts/migrate.sh`:

```bash
#!/bin/bash
set -euo pipefail

echo "Running database migrations..."

# Use kubectl to exec into the database pod
kubectl exec -it deploy/postgres -- psql -U app -d hello_app -f /migrations/001_init.sql

echo "Migrations complete!"
```

### Pattern 3: Integration Tests After Deploy

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      after:
        - host:
            command: ["sh", "-c", "sleep 5 && curl -sf http://localhost:8080/health || exit 1"]
```

### Pattern 4: Notify on Deploy

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      after:
        - host:
            command: ["sh", "-c", "echo '🚀 Deploy complete at $(date)' | tee -a deploy.log"]
```

---

## 8.6 — Custom Actions

Custom actions are user-defined tasks you can run alongside the main Skaffold pipeline. Unlike hooks (which are tied to build/deploy stages), custom actions are standalone and invoked explicitly.

### Defining Custom Actions

```yaml
customActions:
  - name: migrate
    executionMode:
      kubernetesCluster: {}
    containers:
      - name: migrate
        image: hello-app
        command: ["/server", "-migrate"]

  - name: seed
    executionMode:
      local: {}
    commands:
      - command: ["sh", "-c", "./scripts/seed.sh"]
```

### Running Custom Actions

```bash
# Run a specific action
skaffold exec migrate

# Run all custom actions
skaffold exec --all
```

### Practical Example: Database Seeder

Add to `skaffold.yaml`:

```yaml
customActions:
  - name: seed-db
    executionMode:
      local: {}
    commands:
      - command: ["sh", "-c", "./scripts/seed.sh"]
        os: [linux, darwin]
```

`scripts/seed.sh`:

```bash
#!/bin/bash
set -euo pipefail

echo "🌱 Seeding development database..."

# Wait for the service to be ready
until curl -sf http://localhost:8080/health > /dev/null 2>&1; do
  echo "  Waiting for service..."
  sleep 2
done

# Seed some test data
curl -X POST http://localhost:8080/api/seed \
  -H "Content-Type: application/json" \
  -d '{"count": 100}'

echo "✅ Seeding complete!"
```

---

## 8.7 — Status Check Configuration

We introduced `statusCheck` in Chapter 4. Here's the deep-dive.

### Full Configuration

```yaml
deploy:
  statusCheck: true
  statusCheckDeadlineSeconds: 300
  tolerateFailuresUntilDeadline: true
  kubectl:
    manifests:
      - k8s/*.yaml
```

### What Each Field Does

| Field | Default | Meaning |
|-------|---------|---------|
| `statusCheck` | `true` | Enable/disable status checks |
| `statusCheckDeadlineSeconds` | `120` | How long to wait for readiness |
| `tolerateFailuresUntilDeadline` | `false` | If `true`, don't fail on transient errors until the deadline |

### When to Increase the Deadline

- **Large images**: If your image is large and Kind takes time to load it
- **Slow startup**: If your Go service connects to a database on startup and retries
- **Init containers**: If you have init containers that run migrations

```yaml
deploy:
  statusCheckDeadlineSeconds: 300   # 5 minutes for slow starts
  tolerateFailuresUntilDeadline: true  # Tolerate CrashLoopBackOff while DB starts
```

### Resource-Level Status Check

You can also configure per-resource timeouts:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
  labels:
    app: hello-app
    skaffold.dev/run-id: static    # Opt out of Skaffold's run-id labeling
  annotations:
    skaffold.dev/status-check-deadline: "300"   # Per-resource deadline
```

---

## 8.8 — Putting It All Together

Here's our evolving `skaffold.yaml` with everything from this chapter:

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: hello-app

build:
  artifacts:
    - image: hello-app
      docker:
        dockerfile: Dockerfile
      dependencies:
        paths:
          - "**/*.go"
          - "go.mod"
          - "go.sum"
        ignore:
          - "**/*_test.go"
  local:
    push: false
    useBuildkit: true

deploy:
  statusCheck: true
  statusCheckDeadlineSeconds: 120
  kubectl:
    manifests:
      - k8s/*.yaml
    hooks:
      before:
        - host:
            command: ["sh", "-c", "echo '🔄 Deploying...'"]
      after:
        - host:
            command: ["sh", "-c", "echo '✅ Deploy complete!'"]

portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080
  - resourceType: deployment
    resourceName: hello-app
    port: 6060
    localPort: 6060

profiles:
  - name: dev
    build:
      artifacts:
        - image: hello-app
          docker:
            dockerfile: Dockerfile.dev
          sync:
            manual:
              - src: "**/*.go"
                dest: /app
    activation:
      - command: dev

  - name: debug
    build:
      artifacts:
        - image: hello-app
          docker:
            dockerfile: Dockerfile.debug
    activation:
      - command: debug
```

---

## 8.9 — What Could Go Wrong?

### ❌ Port already in use

**Symptom:**
```
port forwarding failed: unable to forward port 8080:
  listen tcp 127.0.0.1:8080: bind: address already in use
```

**Cause:** Another process is using that port.
**Fix:**
```bash
# Find what's using the port
lsof -i :8080 | grep LISTEN
# or on Linux:
ss -tlnp | grep 8080

# Kill it (if safe)
kill <PID>

# Or change the local port in skaffold.yaml:
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 9090    # Use a different local port
```

### ❌ Hook fails and blocks the pipeline

**Symptom:** `skaffold dev` hangs on "Running pre-deploy hook."
**Cause:** The hook command hangs (waiting for input, infinite loop, network timeout).
**Fix:**
1. Add a timeout to your hook commands:
   ```bash
   timeout 30 ./scripts/migrate.sh
   ```
2. Use verbose logging to debug:
   ```bash
   skaffold dev -v debug
   ```
3. Test the hook command manually before adding it to Skaffold.

### ❌ Logs are too noisy / interleaved

**Symptom:** Multiple containers' logs are mixed together and hard to read.
**Fix:**
```bash
# Disable Skaffold's log tailing
skaffold dev --tail=false

# Use stern for better log management
stern hello-app --output json
stern . --namespace default --selector app=hello-app
```

### ❌ Status check times out but the app is healthy

**Symptom:**
```
deployment/hello-app failed. Error: deadline exceeded.
```
But when you check manually: `kubectl get pods` shows the pod is `Running`.

**Cause:** The readiness probe is misconfigured or the initial delay is too long.
**Fix:**
1. Check your readiness probe:
   ```yaml
   readinessProbe:
     httpGet:
       path: /health    # ← Is this endpoint returning 200?
       port: 8080       # ← Is this the right port?
     initialDelaySeconds: 3
   ```
2. Increase the Skaffold deadline:
   ```yaml
   deploy:
     statusCheckDeadlineSeconds: 300
     tolerateFailuresUntilDeadline: true
   ```

### ❌ Port forward stops working after redeploy

**Symptom:** After Skaffold rebuilds and redeploys, `localhost:8080` stops responding.
**Cause:** The old port forward was tied to a pod that got replaced. Skaffold should re-establish it, but sometimes there's a brief gap.
**Fix:** This is usually transient (1–2 seconds). If it persists, press `Ctrl+C` and restart `skaffold dev`. If it happens consistently, file a Skaffold bug report.

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **Port forwarding** | Automatic localhost access to K8s services; supports multiple ports and resource types |
| **Log tailing** | Built-in; use `--tail=false` + `stern` for production-grade filtering |
| **Lifecycle hooks** | Run commands before/after build and deploy; host or container execution |
| **Custom actions** | Standalone tasks (`skaffold exec`) for migrations, seeding, etc. |
| **Status checks** | Configurable deadlines and tolerance for deploy verification |

---

**← [Chapter 7: Profiles & Multi-Config](07-profiles.md)** | **[Chapter 9: CI/CD Integration & The Outer Loop →](09-cicd.md)**
