# Chapter 6 — Debugging Go in Kubernetes with Skaffold

> *"A debugger is worth a thousand print statements."*

---

## What You'll Learn

- How `skaffold debug` works under the hood
- How Skaffold auto-injects **Delve** (the Go debugger) into your container
- Connecting **VS Code** and **GoLand** to a remote Delve session
- Multi-container debugging
- Go-specific debugging gotchas: optimisations, stripped binaries, CGO

---

## 6.1 — What `skaffold debug` Does

`skaffold debug` is like `skaffold dev`, but it **instruments your container** for debugging:

```
skaffold dev                          skaffold debug
┌───────────────────┐                 ┌───────────────────┐
│ Build image       │                 │ Build image       │
│ (normal)          │                 │ (debug flags)     │
│                   │                 │ -gcflags='...'    │
├───────────────────┤                 ├───────────────────┤
│ Deploy            │                 │ Deploy            │
│ (normal)          │                 │ (modified)        │
│                   │                 │ - Injects Delve   │
│                   │                 │ - Adds debug port │
│                   │                 │ - Changes CMD     │
├───────────────────┤                 ├───────────────────┤
│ Port forward      │                 │ Port forward      │
│ (app port only)   │                 │ (app + debug port)│
└───────────────────┘                 └───────────────────┘
```

Specifically, `skaffold debug` does three things automatically:

### 1. Modifies the Build

Skaffold sets the Go compiler flags to **disable optimisations**:

```
-gcflags='all=-N -l'
```

| Flag | Meaning |
|------|---------|
| `-N` | Disable optimisations |
| `-l` | Disable inlining |

Without these flags, the Go compiler may optimise away variables, reorder code, or inline functions — making it impossible to set breakpoints on specific lines or inspect variables.

### 2. Injects the Delve Debugger

Skaffold modifies the container's **entrypoint** to start Delve instead of your binary:

**Before (`skaffold dev`):**
```
ENTRYPOINT ["/server"]
```

**After (`skaffold debug`):**
```
ENTRYPOINT ["dlv", "exec", "/server",
  "--headless",
  "--listen=:56268",
  "--api-version=2",
  "--accept-multiclient",
  "--log"]
```

Delve wraps your binary, exposing a debug API on port `56268`.

### 3. Exposes the Debug Port

Skaffold automatically forwards the debug port (default: `56268`) to your local machine. You don't need to configure this — it just works.

```
Port forwarding service/hello-app in namespace default,
  remote port 8080 -> http://127.0.0.1:8080
Port forwarding pod/hello-app-xxx in namespace default,
  remote port 56268 -> http://127.0.0.1:56268     ← Debug port
```

---

## 6.2 — Running `skaffold debug`

### Start the Debug Session

```bash
skaffold debug --port-forward
```

Wait for the output:

```
Listing files to watch...
 - hello-app
Building [hello-app]...
Tags used in deployment:
 - hello-app -> hello-app:debug-abc123
Starting deploy...
 - deployment.apps/hello-app created
 - service/hello-app created
Waiting for deployments to stabilize...
 - deployment/hello-app is ready.
Port forwarding pod/hello-app-xxx in namespace default,
  remote port 56268 -> address 127.0.0.1:56268
Port forwarding service/hello-app in namespace default,
  remote port 8080 -> http://127.0.0.1:8080
```

Your application is now running inside Delve, waiting for a debugger to connect.

> **Note:** The application **won't respond to HTTP requests** until a debugger connects and continues execution (unless you use the `--continue` flag with Delve).

---

## 6.3 — Connecting VS Code

### Prerequisites

Install the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.Go) for VS Code.

### Create a Launch Configuration

Add to `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Skaffold Debug",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "/app",
      "port": 56268,
      "host": "127.0.0.1",
      "showLog": true,
      "trace": "verbose",
      "substitutePath": [
        {
          "from": "${workspaceFolder}",
          "to": "/app"
        }
      ]
    }
  ]
}
```

**Configuration breakdown:**

| Field | Value | Why |
|-------|-------|-----|
| `type` | `"go"` | Use the Go debug adapter |
| `request` | `"attach"` | Connect to an existing debugger (not launch a new one) |
| `mode` | `"remote"` | Delve is running in a remote container |
| `remotePath` | `"/app"` | Path to source code inside the container (matches Dockerfile WORKDIR) |
| `port` | `56268` | Default Delve port that Skaffold forwards |
| `substitutePath` | Workspace → /app | Maps local file paths to container paths for breakpoints |

### Start Debugging

1. Run `skaffold debug --port-forward` in your terminal
2. In VS Code, open `main.go`
3. Set a breakpoint on the line inside `handleRoot` (click in the gutter)
4. Press **F5** or go to **Run → Start Debugging**
5. In another terminal: `curl http://localhost:8080/`
6. VS Code should hit your breakpoint!

**What you can do while paused:**
- **Inspect variables** — hover over variables to see their values
- **Watch expressions** — add expressions to the Watch panel
- **Call stack** — see the goroutine stack trace
- **Step through code** — F10 (step over), F11 (step into), Shift+F11 (step out)
- **Goroutines** — see all running goroutines and switch between them

---

## 6.4 — Connecting GoLand (JetBrains)

### Create a Run Configuration

1. Go to **Run → Edit Configurations**
2. Click **+** → **Go Remote**
3. Configure:
   - **Host:** `localhost`  
   - **Port:** `56268`
   - **Project path mapping:** `<your local project path>` → `/app`
4. Click **OK**

### Start Debugging

1. Run `skaffold debug --port-forward`
2. Set breakpoints in your Go files
3. Click the **Debug** button (bug icon) next to your run configuration
4. Trigger a request: `curl http://localhost:8080/`
5. GoLand pauses at your breakpoint

---

## 6.5 — Debugging with the Delve CLI

If you prefer the terminal, you can connect Delve directly:

```bash
# Connect to the remote Delve process
dlv connect localhost:56268
```

Useful Delve commands:

```
(dlv) break main.handleRoot        # Set a breakpoint
(dlv) continue                      # Run until breakpoint
(dlv) next                          # Step over
(dlv) step                          # Step into
(dlv) print resp                    # Print a variable
(dlv) goroutines                    # List all goroutines
(dlv) goroutine 1                   # Switch to goroutine 1
(dlv) stack                         # Show stack trace
(dlv) exit                          # Disconnect
```

---

## 6.6 — Debugging Configuration in `skaffold.yaml`

You can customise the debug behaviour:

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

# Debug-specific configuration
profiles:
  - name: debug
    build:
      artifacts:
        - image: hello-app
          docker:
            dockerfile: Dockerfile.debug
            buildArgs:
              GCFLAGS: "all=-N -l"
    activation:
      - command: debug
```

### Custom Debug Dockerfile

For more control, create a `Dockerfile.debug`:

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

# Install Delve
RUN go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build WITHOUT stripping debug info and WITH debug flags
# Notice: NO -ldflags="-s -w" (we NEED debug symbols)
# -gcflags="all=-N -l" disables optimisations for debugging
ARG GCFLAGS="all=-N -l"
RUN CGO_ENABLED=0 GOOS=linux go build \
  -gcflags="${GCFLAGS}" \
  -o /app/server .

# Use a full image (not distroless) so Delve can run
FROM golang:1.23-alpine

# Copy Delve from builder
COPY --from=builder /go/bin/dlv /usr/local/bin/dlv

# Copy the debuggable binary
COPY --from=builder /app/server /server

EXPOSE 8080 56268

ENTRYPOINT ["dlv", "exec", "/server", \
  "--headless", \
  "--listen=:56268", \
  "--api-version=2", \
  "--accept-multiclient", \
  "--continue", \
  "--log"]
```

**Key differences from the production Dockerfile:**

| Production | Debug |
|-----------|-------|
| `-ldflags="-s -w"` (strip symbols) | No stripping |
| No `-gcflags` | `-gcflags="all=-N -l"` |
| `distroless` runtime | Full Alpine (Delve needs a shell) |
| `ENTRYPOINT ["/server"]` | `ENTRYPOINT ["dlv", "exec", ...]` |

The `--continue` flag tells Delve to start executing immediately instead of waiting for a debugger — your app serves requests normally until a debugger attaches.

---

## 6.7 — Multi-Container Debugging

If your project has multiple services (e.g., API + Worker), you can debug them simultaneously:

```yaml
build:
  artifacts:
    - image: api-service
      docker:
        dockerfile: Dockerfile
    - image: worker-service
      docker:
        dockerfile: worker/Dockerfile
```

When you run `skaffold debug`, Skaffold injects Delve into **every** container that it detects as a Go application. Each container gets its own debug port:

```
Port forwarding pod/api-service-xxx    remote port 56268 -> 127.0.0.1:56268
Port forwarding pod/worker-service-xxx remote port 56268 -> 127.0.0.1:56269
```

Create separate VS Code launch configurations for each:

```json
{
  "configurations": [
    {
      "name": "Debug API",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "port": 56268
    },
    {
      "name": "Debug Worker",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "port": 56269
    }
  ]
}
```

Or use a **compound launch configuration** to debug both simultaneously:

```json
{
  "compounds": [
    {
      "name": "Debug All Services",
      "configurations": ["Debug API", "Debug Worker"]
    }
  ]
}
```

---

## 6.8 — What Could Go Wrong?

### ❌ Debugger connects but breakpoints are greyed out / not hit

**Symptom:** VS Code shows "Unverified breakpoint" or breakpoints are never triggered.  
**Cause:** Path mapping is wrong — VS Code can't match your local files to the container paths.  
**Fix:** Verify `substitutePath` in `launch.json`:
```json
"substitutePath": [
  {
    "from": "${workspaceFolder}",
    "to": "/app"
  }
]
```

The `"to"` value must match the `WORKDIR` in your Dockerfile.

### ❌ `could not attach to pid`: debug binary is optimised

**Symptom:** Delve complains about optimised code.  
**Cause:** The binary was built without `-gcflags="all=-N -l"`.  
**Fix:** When using `skaffold debug`, Skaffold should add these flags automatically. If using a custom Dockerfile, ensure:
```dockerfile
RUN go build -gcflags="all=-N -l" -o /app/server .
```

Do **not** use `-ldflags="-s -w"` in debug builds — this strips the symbol table that Delve needs.

### ❌ Port 56268 is already in use

**Symptom:**
```
port forwarding failed: port 56268 already in use
```

**Cause:** A previous debug session didn't clean up, or another process uses that port.  
**Fix:**
```bash
# Find and kill the process
lsof -i :56268 | grep LISTEN
kill <PID>

# Or use a different port
# In your Dockerfile.debug:
ENTRYPOINT ["dlv", "exec", "/server", "--listen=:40000", ...]
```

### ❌ Debug build takes forever

**Symptom:** Every debug build is a full rebuild.  
**Cause:** The `GCFLAGS` build arg changes, invalidating Docker's cache.  
**Fix:** Use BuildKit cache mounts (Chapter 3) in your debug Dockerfile:
```dockerfile
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /app/server .
```

### ❌ Can't step into standard library code

**Symptom:** F11 (step into) doesn't enter `fmt.Println` or `http.ListenAndServe`.  
**Cause:** The Go standard library source isn't available inside the container.  
**Fix:** This is expected in distroless/scratch images. If you need stdlib debugging, use `golang:*-alpine` as the runtime image (as in `Dockerfile.debug`).

### ❌ Goroutine debugging is confusing

**Symptom:** There are dozens of goroutines and you can't find yours.  
**Tip:** In the VS Code debug panel, look at the **GOROUTINES** section. Your HTTP handler runs in a goroutine created by `net/http`. Look for goroutines whose stack trace includes `main.handleRoot`.

In Delve CLI:
```
(dlv) goroutines -t     # Show goroutines with their stacks
```

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **`skaffold debug`** | Auto-injects Delve, disables Go optimisations, exposes debug port |
| **Compiler flags** | `-gcflags="all=-N -l"` is essential; never strip symbols in debug builds |
| **VS Code** | Use `attach` mode with `substitutePath` mapping |
| **GoLand** | Use **Go Remote** run configuration |
| **Multi-container** | Each container gets its own debug port; use compound launch configs |
| **`--continue`** | Let the app run normally until a debugger attaches |

---

**← [Chapter 5: The Dev Loop](05-dev-loop.md)** | **[Chapter 7: Profiles, Environments & Multi-Config →](07-profiles.md)**
