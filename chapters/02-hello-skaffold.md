# Chapter 2 — Hello, Skaffold: Your First Go Service on Kubernetes

> *"The journey of a thousand microservices begins with a single `skaffold dev`."*

---

## What You'll Learn

- How to set up **Kind** as your local Kubernetes cluster
- How to write a simple Go HTTP server for our sample project
- How to create a **multi-stage Dockerfile** optimised for Go
- How to write your first Kubernetes **Deployment** and **Service** manifests
- How to create a minimal `skaffold.yaml` and run `skaffold dev`
- The difference between `skaffold dev` and `skaffold run`

---

## 2.1 — Prerequisites Check

Before we write any code, let's verify that all required tools are installed. Open your terminal and run each command:

```bash
# Go
go version
# Expected: go version go1.21+ (or later)

# Docker
docker version
# Expected: Client and Server both responding

# kubectl
kubectl version --client
# Expected: Client Version: v1.28+ (or later)

# Kind
kind version
# Expected: kind v0.20+ (or later)

# Skaffold
skaffold version
# Expected: v2.10+ (or later)
```

> **Don't have these installed?** Check the [README](../README.md) for download links.

---

## 2.2 — Create Your Local Kubernetes Cluster

We'll use **Kind** (Kubernetes IN Docker) because it's the lightest option and works identically across operating systems.

### Create the Cluster

```bash
kind create cluster --name skaffold-lab
```

You should see output like:

```
Creating cluster "skaffold-lab" ...
 ✓ Ensuring node image (kindest/node:v1.31.0) 🖼
 ✓ Preparing nodes 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
Set kubectl context to "kind-skaffold-lab"
```

### Verify the Cluster

```bash
kubectl cluster-info --context kind-skaffold-lab
```

```
Kubernetes control plane is running at https://127.0.0.1:XXXXX
CoreDNS is running at https://127.0.0.1:XXXXX/api/v1/...
```

### What Just Happened?

Kind created a Docker container that runs a full Kubernetes cluster inside it. You can see it:

```bash
docker ps
```

```
CONTAINER ID   IMAGE                  COMMAND                  NAMES
a1b2c3d4e5f6   kindest/node:v1.31.0   "/usr/local/bin/entr…"   skaffold-lab-control-plane
```

That single container is your entire Kubernetes cluster: API server, etcd, scheduler, controller manager, kubelet — all running inside one Docker container. This is why Kind is so fast to set up.

> **K8s Concept: Context**  
> `kubectl` uses **contexts** to know which cluster to talk to. When Kind created the cluster, it set your context to `kind-skaffold-lab`. You can see all contexts with `kubectl config get-contexts` and switch with `kubectl config use-context <name>`.

---

## 2.3 — Bootstrap the Go Project

Let's create our sample project. We'll build a simple HTTP server that responds with a JSON greeting.

### Initialise the Go Module

```bash
cd project/
go mod init github.com/skaffold-masterclass/hello-app
```

### Write `main.go`

```go
// main.go
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
```

**Why this structure?**

- **`/` endpoint** — Returns the pod hostname, which lets us see which Pod is serving requests when we scale up later
- **`/health` endpoint** — Kubernetes liveness/readiness probes will hit this (covered in [Chapter 4](04-deploy-pipeline.md))
- **`PORT` env var** — Following the [12-Factor App](https://12factor.net/port-binding) convention of configurable ports
- **Structured logging** — Using `log.Printf` with emojis for easy visual scanning in `skaffold dev` output

### Test Locally (Without Kubernetes)

Before we containerise, let's prove the code works:

```bash
go run main.go
```

In another terminal:

```bash
curl http://localhost:8080/
```

```json
{"message":"Hello from Skaffold! 🎉","hostname":"your-machine-name","timestamp":"2026-02-13T12:00:00+05:30"}
```

Great. Stop the server with `Ctrl+C`. Now let's containerise it.

---

## 2.4 — Write the Dockerfile

This is where Go developers often get things wrong. A naive Dockerfile produces a **900 MB+ image**. We'll use a **multi-stage build** to produce a **~10 MB image**.

```dockerfile
# ---- Build Stage ----
# Use the official Go image as the build environment.
# This image is ~800 MB — we only use it to compile.
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container.
WORKDIR /app

# Copy go.mod and go.sum first.
# Docker caches layers — if these files haven't changed,
# Docker will reuse the cached 'go mod download' layer.
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the source code.
# This layer is invalidated every time you change a .go file.
COPY . .

# Build the Go binary.
# CGO_ENABLED=0  — Produce a fully static binary (no C dependencies)
# -ldflags="-s -w" — Strip debug info and symbol table (smaller binary)
# -o /app/server   — Output path for the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server .

# ---- Runtime Stage ----
# Use a minimal base image for the final container.
# 'gcr.io/distroless/static-debian12' contains NOTHING except:
#   - CA certificates (for HTTPS calls)
#   - /etc/passwd (for nonroot user)
#   - timezone data
# Total size: ~2 MB
FROM gcr.io/distroless/static-debian12

# Copy ONLY the compiled binary from the build stage.
COPY --from=builder /app/server /server

# Document (but don't enforce) the port this container listens on.
EXPOSE 8080

# Run as a non-root user for security.
USER nonroot:nonroot

# Start the server.
ENTRYPOINT ["/server"]
```

### Why Multi-Stage?

| Stage | Base Image | Size | Purpose |
|-------|-----------|------|---------|
| Builder | `golang:1.23-alpine` | ~300 MB | Compile the Go binary |
| Runtime | `distroless/static` | ~2 MB | Run the binary |

The final image contains **only** the compiled binary and the distroless base — no Go toolchain, no source code, no package manager.

### Why Not `scratch`?

You might see tutorials using `FROM scratch` (a completely empty image). We use `distroless` instead because:

1. It includes **CA certificates** — needed if your app makes HTTPS calls
2. It includes a **nonroot user** — better security practices
3. It includes **timezone data** — `time.Now()` works correctly

If your app truly needs nothing, `scratch` saves ~1 MB. For most real applications, `distroless` is the better default.

### Create `go.sum`

Since our project has no external dependencies, we still need a `go.sum` file for Docker:

```bash
go mod tidy
```

This creates an empty `go.sum` (or populates it if you add dependencies later).

---

## 2.5 — Write the Kubernetes Manifests

We need two Kubernetes resources: a **Deployment** (to run our Pods) and a **Service** (to expose them).

### `k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
  labels:
    app: hello-app
spec:
  replicas: 1                    # Start with one Pod
  selector:
    matchLabels:
      app: hello-app             # This Deployment manages Pods with this label
  template:                      # Pod template — what each Pod looks like
    metadata:
      labels:
        app: hello-app           # Labels applied to each Pod
    spec:
      containers:
        - name: hello-app
          image: hello-app       # Skaffold will replace this with the built image tag
          ports:
            - containerPort: 8080
          env:
            - name: PORT
              value: "8080"
          livenessProbe:          # K8s checks: is the container alive?
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
          readinessProbe:         # K8s checks: is the container ready for traffic?
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5
```

**Let's break this down for K8s newcomers:**

- **`replicas: 1`** — How many identical Pods to run. We start with 1; we'll scale later.
- **`selector.matchLabels`** — Tells the Deployment which Pods belong to it. It looks for Pods with the label `app: hello-app`.
- **`template`** — A blueprint for creating Pods. Every Pod created by this Deployment will look like this.
- **`image: hello-app`** — This is a placeholder! Skaffold will auto-replace this with the actual image tag (e.g., `hello-app:abc123def`). This is called **image replacement** and is one of Skaffold's key features.
- **`livenessProbe`** — Kubernetes pings `/health` every 10 seconds. If the probe fails 3 times, K8s restarts the container.
- **`readinessProbe`** — Similar to liveness, but determines if the Pod should receive traffic. If it fails, K8s removes the Pod from the Service's endpoint list.

### `k8s/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: hello-app
spec:
  type: ClusterIP              # Only accessible inside the cluster (default)
  selector:
    app: hello-app             # Route traffic to Pods with this label
  ports:
    - port: 8080               # The port the Service listens on
      targetPort: 8080         # The port on the container to forward to
      protocol: TCP
```

**K8s Concept: Service Types**

| Type | Accessible From | Use Case |
|------|----------------|----------|
| `ClusterIP` | Inside the cluster only | Default; use with `port-forward` for local dev |
| `NodePort` | Outside the cluster via `<NodeIP>:<NodePort>` | Quick external access |
| `LoadBalancer` | External via cloud load balancer | Production on cloud providers |

We use `ClusterIP` because Skaffold's `portForward` will handle external access for us.

---

## 2.6 — Write `skaffold.yaml`

Now, the star of the show:

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
  local:
    push: false                # Don't push to a registry; Kind loads images directly

deploy:
  kubectl:
    manifests:
      - k8s/*.yaml

portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080
```

**Let's dissect each stanza:**

### `build`

```yaml
build:
  artifacts:
    - image: hello-app         # The name Skaffold uses to tag the image
      docker:
        dockerfile: Dockerfile  # Path to the Dockerfile (relative to skaffold.yaml)
  local:
    push: false                # ← KEY SETTING for Kind
```

- **`artifacts`** — A list of images to build. Each artifact maps to one Dockerfile.
- **`image: hello-app`** — This name must match the `image:` field in your Kubernetes manifests. Skaffold finds matching image names and replaces them with the built tag.
- **`local.push: false`** — This is critical for Kind clusters. Instead of pushing to a registry, Skaffold uses `kind load docker-image` to load the image directly into the cluster. This skips the entire push/pull cycle and saves significant time.

### `deploy`

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml            # Glob pattern matching all YAML files in k8s/
```

- **`kubectl`** — Use raw `kubectl apply` as the deployer. This is the simplest option and perfect for getting started.
- **`manifests`** — The files to apply. Skaffold reads these, replaces image tags, and runs `kubectl apply -f`.

### `portForward`

```yaml
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080                 # The Service port
    localPort: 8080            # The port on YOUR machine
```

This tells Skaffold: *"After deploying, forward port 8080 from the `hello-app` Service to `localhost:8080` on my machine."*

Without this, you'd have to manually run `kubectl port-forward svc/hello-app 8080:8080` every time.

---

## 2.7 — Your First `skaffold dev`

### Project Structure Check

Your project directory should now look like this:

```
project/
├── main.go
├── go.mod
├── go.sum
├── Dockerfile
├── skaffold.yaml
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

### Run It

```bash
skaffold dev
```

**What you'll see** (annotated):

```
Listing files to watch...                    # 1. Skaffold scans your directory
 - hello-app
Generating tags...                           # 2. Creates a unique image tag
 - hello-app -> hello-app:v2.10.0-22-g...
Checking cache...                            # 3. Checks if the image is already built
 - hello-app: Not found. Building       
Found [kind-skaffold-lab] context...         # 4. Detects your Kind cluster
Building [hello-app]...                      # 5. Runs `docker build`
[+] Building 12.4s (12/12) FINISHED
 => [builder 1/5] FROM golang:1.23-alpine
 => ...
 => [builder 5/5] RUN CGO_ENABLED=0 ...
 => [stage-1 1/1] COPY --from=builder ...
Loading images into kind cluster nodes...    # 6. Loads image into Kind (no push!)
 - hello-app:abc123 -> Loaded
Tags used in deployment:
 - hello-app -> hello-app:abc123@sha256:... 
Starting deploy...                           # 7. Runs kubectl apply
 - deployment.apps/hello-app created
 - service/hello-app created
Waiting for deployments to stabilize...      # 8. Status check
 - deployment/hello-app is ready.
Port forwarding service/hello-app in         # 9. Sets up port forwarding
  namespace default, remote port 8080 ->
  http://127.0.0.1:8080
Press Ctrl+C to exit
[hello-app] 🚀 Server starting on port 8080  # 10. Your Go app's log output!
```

### Test It

In another terminal:

```bash
curl http://localhost:8080/
```

```json
{
  "message": "Hello from Skaffold! 🎉",
  "hostname": "hello-app-7b9f5c6d8-x2k4m",
  "timestamp": "2026-02-13T07:00:00Z"
}
```

Notice the `hostname` — that's the **Kubernetes Pod name**, not your machine's hostname. Your Go code is running inside a container, inside a Pod, inside your Kind cluster. Skaffold is forwarding the traffic transparently.

### Make a Change (The Magic Moment)

With `skaffold dev` still running, edit `main.go`:

```go
// Change the message
Message: "Hello from Skaffold! 🎉 (v2 — live update!)",
```

**Save the file.** Watch the terminal:

```
Generating tags...
 - hello-app -> hello-app:v2.10.0-23-g...
Building [hello-app]...
[+] Building 2.1s (12/12) FINISHED           # ← Much faster! Docker cache hit
Loading images into kind cluster nodes...
Starting deploy...
 - deployment.apps/hello-app configured      # "configured" not "created"
Waiting for deployments to stabilize...
 - deployment/hello-app is ready.
[hello-app] 🚀 Server starting on port 8080
```

```bash
curl http://localhost:8080/
```

```json
{
  "message": "Hello from Skaffold! 🎉 (v2 — live update!)",
  "hostname": "hello-app-8c4f7d8e9-q3j5n",
  "timestamp": "2026-02-13T07:01:15Z"
}
```

**That's Skaffold's Inner Loop in action.** You edited a file, and within seconds your change was live in Kubernetes. No manual docker build, no kubectl apply, no port-forward setup.

---

## 2.8 — `skaffold dev` vs `skaffold run`

Now stop `skaffold dev` with `Ctrl+C`:

```
Cleaning up...
 - deployment.apps "hello-app" deleted
 - service "hello-app" deleted
```

**Skaffold cleaned up after itself.** The Deployment and Service are deleted. This is the default behaviour of `skaffold dev` — it's meant for temporary development sessions.

Now try `skaffold run`:

```bash
skaffold run --tail
```

This builds and deploys just like `skaffold dev`, but:
- It does **not** watch for file changes
- It does **not** clean up when you stop it
- The `--tail` flag streams logs (otherwise it exits silently after deploy)

To clean up manually:

```bash
skaffold delete
```

This reads `skaffold.yaml` and deletes all the Kubernetes resources it deployed.

| Behaviour | `skaffold dev` | `skaffold run` |
|-----------|---------------|----------------|
| Watches for file changes | ✅ | ❌ |
| Cleans up on Ctrl+C | ✅ | ❌ |
| Port forwarding | ✅ (with `portForward`) | ❌ (unless `--port-forward`) |
| Log tailing | ✅ (automatic) | ❌ (unless `--tail`) |
| Use case | Active development | One-shot deploy, CI |

---

## 2.9 — Project File Summary

Here's every file you created in this chapter, for reference:

### Directory Layout

```
project/
├── main.go              # Go HTTP server
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
├── Dockerfile           # Multi-stage Docker build
├── skaffold.yaml        # Skaffold configuration
└── k8s/
    ├── deployment.yaml  # Kubernetes Deployment
    └── service.yaml     # Kubernetes Service
```

---

## 2.10 — What Could Go Wrong?

### ❌ `Cannot connect to the Docker daemon`

**Symptom:**
```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock
```

**Cause:** Docker Desktop/Engine isn't running.  
**Fix:** Start Docker Desktop, or run `sudo systemctl start docker` on Linux.

### ❌ `error: no context exists with the name: "kind-skaffold-lab"`

**Symptom:** Skaffold can't find your Kind cluster.  
**Cause:** The Kind cluster wasn't created, or the kubectl context wasn't set.  
**Fix:**
```bash
# Check existing clusters
kind get clusters

# If empty, create one
kind create cluster --name skaffold-lab

# Set the context
kubectl config use-context kind-skaffold-lab
```

### ❌ `ErrImageNeverPull` or `ImagePullBackOff`

**Symptom:** Pods crash with image pull errors.  
**Cause:** Skaffold tried to push the image to a registry, but your `local.push` is set to `true` (or missing), and there's no registry.  
**Fix:** Ensure `skaffold.yaml` has:
```yaml
build:
  local:
    push: false
```

For Kind, images must be loaded directly (Skaffold does this automatically when `push: false`).

### ❌ `CrashLoopBackOff`

**Symptom:**
```
NAME                        READY   STATUS             RESTARTS
hello-app-xxx               0/1     CrashLoopBackOff   3
```

**Cause:** Your Go binary is crashing immediately. Common reasons:
- Port conflict inside the container
- Missing environment variable
- Binary compiled for wrong OS/architecture

**Debug steps:**
```bash
# Check pod logs
kubectl logs deployment/hello-app

# Check pod events
kubectl describe pod -l app=hello-app
```

### ❌ `Skaffold build takes forever`

**Symptom:** Every build downloads all Go modules.  
**Cause:** Your Dockerfile doesn't cache the `go mod download` layer properly.  
**Fix:** Ensure `COPY go.mod go.sum` comes **before** `COPY . .` in your Dockerfile. This way, Docker caches the module download layer and only re-downloads when dependencies change. We'll make this even faster with BuildKit cache mounts in [Chapter 3](03-build-pipeline.md).

### ❌ `Port 8080 is already in use`

**Symptom:**
```
port forwarding service/hello-app: error forwarding port 8080
```

**Cause:** Another process is using port 8080.  
**Fix:**
```bash
# Find what's using port 8080
lsof -i :8080

# Either kill that process or change the local port in skaffold.yaml:
portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 9090      # Use a different local port
```

---

## Summary

| What You Did | Why It Matters |
|-------------|---------------|
| Created a Kind cluster | Local Kubernetes — no cloud costs, instant setup |
| Wrote `main.go` | A real Go HTTP server, not a toy example |
| Created a multi-stage Dockerfile | ~10 MB image instead of ~900 MB |
| Wrote K8s Deployment + Service | Your app runs in Pods with health checks and stable networking |
| Created `skaffold.yaml` | One file connects build → deploy → port-forward |
| Ran `skaffold dev` | Experienced the automated Inner Loop firsthand |
| Made a live change | Saw the rebuild-redeploy cycle in < 5 seconds |

You now have a working Skaffold development environment. In the next chapter, we'll go deep on the **Build Pipeline** — how to squeeze maximum performance out of Docker builds for Go, including BuildKit cache mounts that can cut rebuild times to under 2 seconds.

---

**← [Chapter 1: The Philosophy](01-philosophy.md)** | **[Chapter 3: The Build Pipeline →](03-build-pipeline.md)**
