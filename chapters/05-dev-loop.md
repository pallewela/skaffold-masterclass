# Chapter 5 — The Dev Loop: File Sync, Hot Reload & Rebuild Triggers

> *"The fastest rebuild is the one that doesn't happen."*

---

## What You'll Learn

- How `skaffold dev` watches, triggers, and rebuilds under the hood
- **File Sync** — push changed files into a running container *without rebuilding the image*
- Using **CompileDaemon** for Go hot-reloading inside a container
- Trigger modes: `polling`, `notify`, and `manual`
- Optimising the dev loop for Go: from 10 seconds to < 1 second per iteration

---

## 5.1 — The `skaffold dev` Pipeline, Up Close

When you run `skaffold dev`, Skaffold enters a continuous loop:

```
┌─────────────────────────────────────────────────────────┐
│                    skaffold dev LOOP                     │
│                                                         │
│   ┌─────────┐     ┌─────────────┐     ┌──────────────┐ │
│   │ WATCH   │────►│  DECISION   │────►│   ACTION     │ │
│   │         │     │             │     │              │ │
│   │ fsnotify│     │ Sync-able?  │     │ A) File Sync │ │
│   │ detects │     │  or         │     │ B) Rebuild   │ │
│   │ change  │     │ Rebuild?    │     │    + Deploy  │ │
│   └─────────┘     └─────────────┘     └──────────────┘ │
│        ▲                                      │         │
│        └──────────────────────────────────────┘         │
└─────────────────────────────────────────────────────────┘
```

**The Decision Point is key.** When a file changes, Skaffold asks:

1. **Is this file covered by a `sync` rule?** → If yes, copy it directly into the running container (fast path — no rebuild)
2. **Is this file in the artifact's dependency list?** → If yes, trigger a full rebuild + redeploy (slow path)
3. **Neither?** → Ignore the change

This is where the dev loop can go from "good" to "incredible."

---

## 5.2 — Understanding File Sync

File Sync copies changed files directly into a running container's filesystem. No Docker build, no image push, no pod restart.

### Why It Matters for Go

At first glance, file sync seems useless for Go — Go is a compiled language. You can't just copy a `.go` file into a container and expect it to work. **But** you *can* combine file sync with an **in-container recompiler** that watches for changes and rebuilds the binary inside the container.

This is the fastest possible dev loop:

| Step | Without File Sync | With File Sync + CompileDaemon |
|------|------------------|-------------------------------|
| File change detected | ~0.1s | ~0.1s |
| Docker build | 2–10s | **Skipped** |
| Load image into Kind | 1–3s | **Skipped** |
| kubectl apply | 1–2s | **Skipped** |
| Pod restart | 2–5s | **Skipped** |
| Go recompile (in-container) | — | 0.5–2s |
| **Total** | **6–20s** | **0.6–2.1s** |

### Sync Modes

Skaffold supports three sync modes:

#### 1. `infer` — Auto-detect sync rules (limited to Jib/Buildpacks)
```yaml
sync:
  infer: []
```
Not useful for Go — this is designed for interpreted languages and special builders.

#### 2. `manual` — Explicit sync rules
```yaml
sync:
  manual:
    - src: "**/*.go"
      dest: /app
      strip: ""
```
You define exactly which files to sync and where. This is what we'll use.

#### 3. `auto` — Skaffold infers from the Dockerfile
```yaml
sync:
  auto: true
```
Skaffold analyses your Dockerfile's `COPY` instructions and creates sync rules automatically. Works well when your Dockerfile copies source into a known location.

---

## 5.3 — Setting Up File Sync with CompileDaemon for Go

Here's the strategy:

1. Change the Dockerfile to include the Go toolchain in the development image (we'll use a **dev profile** to keep the production multi-stage build separate)
2. Use [CompileDaemon](https://github.com/githubnemo/CompileDaemon) to watch for `.go` file changes inside the container and recompile
3. Configure Skaffold `sync` to push `.go` files directly into the container

### Step 1: Create a Development Dockerfile

Create `Dockerfile.dev`:

```dockerfile
# Dockerfile.dev — Development image with Go toolchain for hot reload

FROM golang:1.23-alpine

# Install CompileDaemon for auto-recompilation
RUN go install github.com/githubnemo/CompileDaemon@latest

WORKDIR /app

# Copy dependency files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy application source
COPY . .

# CompileDaemon watches for file changes, recompiles, and restarts the binary
# -build        → The build command to run
# -command      → The command to run after building
# -directory     → The directory to watch
# -pattern       → File pattern to watch
# -graceful-kill → Send SIGTERM instead of SIGKILL
ENTRYPOINT ["CompileDaemon", \
  "-build=go build -o /app/server .", \
  "-command=/app/server", \
  "-directory=/app", \
  "-pattern=(.+\\.go)$", \
  "-graceful-kill=true", \
  "-log-prefix=false"]
```

### Step 2: Update `skaffold.yaml` with a Dev Profile

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: hello-app

# Default (production) build
build:
  artifacts:
    - image: hello-app
      docker:
        dockerfile: Dockerfile
  local:
    push: false

deploy:
  kubectl:
    manifests:
      - k8s/*.yaml

portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080

# Development profile with file sync
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
      - command: dev              # Auto-activates on `skaffold dev`
```

### Step 3: Run with the Dev Profile

```bash
skaffold dev -p dev
```

Or, since we added `activation: command: dev`, Skaffold auto-activates the `dev` profile when using `skaffold dev`:

```bash
skaffold dev     # Automatically uses the 'dev' profile
```

### What Happens Now

1. Skaffold starts with the `dev` profile active
2. Builds the image using `Dockerfile.dev` (includes Go toolchain + CompileDaemon)
3. Deploys the pod with CompileDaemon as the entrypoint
4. When you edit a `.go` file:
   - Skaffold detects the change
   - Sees it matches the `sync.manual` rule (`**/*.go` → `/app`)
   - **Copies the file directly into the running container** (no Docker build!)
   - CompileDaemon detects the new file, recompiles, and restarts the binary

```
[hello-app] 2026/02/13 12:30:00 Running build command!
[hello-app] 2026/02/13 12:30:01 Build ok.
[hello-app] 2026/02/13 12:30:01 Hard-restarting process...
[hello-app] 🚀 Server starting on port 8080
```

**Total time from file save to running new code: ~1–2 seconds.**

---

## 5.4 — Trigger Modes

Skaffold supports three ways to detect file changes:

### `notify` (Default)

```yaml
build:
  local:
    push: false
# No special configuration needed — notify is the default
```

Uses OS-level file system notifications (`inotify` on Linux, `FSEvents` on macOS). Most efficient — zero CPU overhead when idle.

### `polling`

```bash
skaffold dev --trigger=polling --watch-poll-interval=500ms
```

Periodically scans the filesystem for changes. Useful when:
- File system notifications don't work (some network-mounted volumes)
- You're on a system with low `inotify` limits

### `manual`

```bash
skaffold dev --trigger=manual
```

Skaffold only rebuilds when you **press Enter** in the terminal. This gives you complete control:

```
Watching for changes...
[Press Enter to trigger rebuild]
```

**When to use manual trigger:**
- When you're making multiple related changes and don't want intermediate rebuilds
- When auto-rebuild is too aggressive and wastes resources
- During debugging when you want to rebuild at specific moments

---

## 5.5 — Managing inotify Limits on Linux

If you're on Linux and get this error:

```
too many open files
```

or

```
User limit of inotify watches reached
```

You've hit the default `inotify` limit. Fix it:

```bash
# Check current limit
cat /proc/sys/fs/inotify/max_user_watches

# Increase temporarily
sudo sysctl fs.inotify.max_user_watches=524288

# Increase permanently
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

---

## 5.6 — Custom Build Scripts

For ultimate control, you can replace Skaffold's build entirely with a custom script:

```yaml
build:
  artifacts:
    - image: hello-app
      custom:
        buildCommand: ./scripts/build.sh
        dependencies:
          paths:
            - "**/*.go"
            - "go.mod"
```

`scripts/build.sh`:

```bash
#!/bin/bash
set -euo pipefail

# Skaffold sets these environment variables:
# $IMAGE  — The fully-tagged image name to build
# $PUSH_IMAGE — Whether to push (true/false)

echo "Building $IMAGE..."

docker build -t "$IMAGE" .

if [ "$PUSH_IMAGE" = "true" ]; then
    docker push "$IMAGE"
fi
```

**Use case:** When you need to run code generation, protobuf compilation, or other pre-build steps that don't fit in a Dockerfile.

---

## 5.7 — Dev Loop Performance Comparison

Here's a comprehensive comparison of all dev loop strategies for our Go project:

| Strategy | Rebuild Time | Image Build? | Pod Restart? | Complexity |
|----------|-------------|-------------|-------------|-----------|
| Full rebuild (`skaffold dev`, default) | 5–20s | ✅ | ✅ | Low |
| BuildKit cache mounts (Ch. 3) | 2–5s | ✅ | ✅ | Low |
| File sync + CompileDaemon | 1–2s | ❌ | ❌ | Medium |
| Manual trigger | On demand | ✅ | ✅ | Low |

**Our recommendation:**

- **Getting started?** Use the default full rebuild with BuildKit cache mounts (Chapters 2–3)
- **Want maximum speed?** Use the `dev` profile with file sync + CompileDaemon (this chapter)
- **Working on complex changes?** Use `--trigger=manual` and rebuild when ready

---

## 5.8 — What Could Go Wrong?

### ❌ File sync doesn't seem to work — full rebuild every time

**Symptom:** After every `.go` change, Skaffold rebuilds the Docker image instead of syncing.  
**Cause:** The `sync` configuration isn't matching your files.  
**Fix:** Debug with verbose output:
```bash
skaffold dev -p dev -v debug
```

Look for:
```
Changed files are not syncable. Rebuilding...
```

This means the changed file doesn't match any sync rule. Check your glob patterns:
```yaml
sync:
  manual:
    - src: "**/*.go"     # Matches .go files in all subdirectories
      dest: /app         # Must match the WORKDIR in your dev Dockerfile
```

### ❌ CompileDaemon: `command not found`

**Symptom:** Pod crashes with "CompileDaemon: not found."  
**Cause:** The `RUN go install` step failed or isn't in the right Dockerfile.  
**Fix:** Ensure `Dockerfile.dev` (not the production `Dockerfile`) includes:
```dockerfile
RUN go install github.com/githubnemo/CompileDaemon@latest
```

And that the dev profile uses `Dockerfile.dev`:
```yaml
profiles:
  - name: dev
    build:
      artifacts:
        - image: hello-app
          docker:
            dockerfile: Dockerfile.dev    # ← Not 'Dockerfile'
```

### ❌ Stale binary after sync

**Symptom:** You sync a `.go` file but the HTTP response still shows old content.  
**Cause:** CompileDaemon's build failed silently, or it's watching the wrong directory.  
**Fix:** Check the pod logs:
```bash
kubectl logs -l app=hello-app -f
```

Look for build errors:
```
Running build command!
./main.go:15:2: undefined: newFunction
Build FAILED.
```

CompileDaemon will keep the old binary running when the build fails. Fix the compile error and save again — it will retry.

### ❌ `Too many open files` on Linux

**Symptom:** Skaffold crashes or file watching stops.  
**Cause:** Hit the `inotify` limit (see section 5.5 above).  
**Fix:** Increase `max_user_watches` to 524288.

### ❌ File sync works, but changes are lost on pod restart

**Symptom:** Synced files disappear if the pod restarts.  
**Cause:** File sync writes to the container's ephemeral filesystem. If the pod restarts (crash, scale event, new deploy), it starts from the original image.  
**This is by design.** File sync is a development convenience, not a persistence mechanism. The synced state only needs to last until the next full rebuild.

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **File Sync** | Push files into running containers without rebuilding. The fastest dev loop. |
| **CompileDaemon** | Watches `.go` files inside the container and recompiles. Pairs perfectly with sync. |
| **Trigger Modes** | `notify` (default), `polling` (fallback), `manual` (control freak). |
| **Dev Profile** | Separate your fast-dev config from your production build. |
| **inotify limits** | On Linux, increase `max_user_watches` to avoid file watching failures. |

---

**← [Chapter 4: The Deploy Pipeline](04-deploy-pipeline.md)** | **[Chapter 6: Debugging Go in Kubernetes →](06-debugging.md)**
