# Chapter 3 — The Build Pipeline: Docker, BuildKit & Go Module Caching

> *"A 2-second rebuild is the difference between flow state and frustration."*

---

## What You'll Learn

- How Skaffold's **build stage** works internally
- Go-specific Dockerfile optimisation: layer ordering, `-ldflags`, `CGO_ENABLED=0`
- **BuildKit cache mounts** — the secret weapon for sub-3-second Go rebuilds
- Skaffold **tag policies**: `sha256`, `gitCommit`, `dateTime`, `inputDigest`
- Alternative builders: **Buildpacks** and **Custom Builders**
- When to use `local`, `cluster`, or `googleCloudBuild` build modes

---

## 3.1 — How Skaffold Builds Images

When Skaffold encounters the `build` stanza, it does the following:

```
┌────────────────────────────────────────────────────────┐
│                    BUILD PIPELINE                       │
│                                                        │
│  1. Determine which artifacts need rebuilding          │
│     (file change detection via checksums)              │
│                                                        │
│  2. Generate a tag for each artifact                   │
│     (tag policy: sha256, gitCommit, etc.)              │
│                                                        │
│  3. Execute the builder for each artifact              │
│     (Docker, Buildpacks, Jib, Bazel, Custom)           │
│                                                        │
│  4. Load or push the image                             │
│     (Kind: load directly | Registry: docker push)      │
│                                                        │
│  5. Return the list of built image:tag pairs            │
│     (passed to the deploy stage for image replacement) │
└────────────────────────────────────────────────────────┘
```

### Artifact Dependency Detection

Skaffold doesn't blindly rebuild everything. It computes a **checksum** of the files that each artifact depends on. If nothing has changed, it skips the build entirely. You'll see this in your terminal:

```
Checking cache...
 - hello-app: Found Locally
```

By default, Skaffold watches all files in the artifact's context directory (the directory containing the Dockerfile). You can control this with `dependencies`:

```yaml
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
          - "**/*_test.go"    # Don't rebuild for test file changes
```

**Why specify dependencies?** If you have documentation, scripts, or test files alongside your source, changes to those files would trigger unnecessary rebuilds. Explicit `paths` and `ignore` rules make your inner loop faster.

---

## 3.2 — Anatomy of a Go-Optimised Dockerfile

Let's revisit our Dockerfile from Chapter 2, but now with **BuildKit cache mounts** — the single biggest performance improvement you can make:

```dockerfile
# syntax=docker/dockerfile:1

# ---- Build Stage ----
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy dependency files first (layer caching)
COPY go.mod go.sum ./

# Download dependencies with a cache mount.
# --mount=type=cache persists the cache directory across builds.
# /go/pkg/mod   → Go module cache
# /root/.cache  → Go build cache
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build with cache mounts for BOTH module and build caches
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -o /app/server .

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/server /server

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

### The `# syntax=docker/dockerfile:1` Directive

This **must** be the first line. It enables BuildKit's extended features (like `--mount`). Without it, the `RUN --mount=...` directives will fail.

### Understanding Cache Mounts

Docker's normal caching works at the **layer level**: if any file in a `COPY` command changes, that layer and all subsequent layers are rebuilt. This means every Go source change triggers a full re-compilation.

**Cache mounts** are different. They persist a directory **across builds**, independent of layer caching:

| Cache Type | Mount Path | Purpose |
|-----------|-----------|---------|
| Module cache | `/go/pkg/mod` | Downloaded Go modules (dependencies) |
| Build cache | `/root/.cache/go-build` | Compiled object files from previous builds |

**The impact is dramatic:**

| Scenario | Without Cache Mounts | With Cache Mounts |
|----------|---------------------|-------------------|
| First build | ~30s | ~30s (no cache yet) |
| Dependency change | ~20s (re-download all) | ~5s (only new deps) |
| Source-only change | ~10s (recompile all) | ~2s (incremental) |

The Go compiler is already fast, but cache mounts let Docker reuse the Go build cache between builds. This means only the **changed packages** are recompiled — exactly like running `go build` locally.

### Go Build Flags Explained

```bash
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server .
```

| Flag | Purpose | Effect |
|------|---------|--------|
| `CGO_ENABLED=0` | Disable CGO (C interop) | Produces a fully static binary; no glibc dependency |
| `GOOS=linux` | Target OS | Ensures Linux binary regardless of host OS |
| `-ldflags="-s -w"` | Strip debug info | `-s` strips symbol table, `-w` strips DWARF. Reduces binary by ~30% |
| `-o /app/server` | Output path | Explicit output location |

**Binary size comparison for our hello-app:**

| Flags | Binary Size |
|-------|------------|
| Default (no flags) | ~7.5 MB |
| `-ldflags="-s -w"` | ~5.2 MB |
| + UPX compression | ~2.1 MB |

We don't recommend UPX for production (it increases startup time), but it's worth knowing about.

---

## 3.3 — Skaffold Tag Policies

Every time Skaffold builds an image, it needs to **tag** it with a unique identifier. The tag policy determines how that identifier is generated.

```yaml
build:
  tagPolicy:
    sha256: {}          # Default
```

### Available Tag Policies

| Policy | Tag Format | When to Use |
|--------|-----------|-------------|
| `sha256` | `hello-app:latest` + digest | **Default.** Simple, always works. Uses content-based addressing. |
| `gitCommit` | `hello-app:abc1234` | When you want tags tied to Git commits. Great for traceability. |
| `dateTime` | `hello-app:2026-02-13_12-30-00` | When you want human-readable, time-sorted tags. |
| `inputDigest` | `hello-app:a1b2c3d4e5` | Tags based on the hash of input files. Guarantees identical inputs → identical tags. |
| `envTemplate` | `hello-app:{{.FOO}}` | When you want to use environment variables in the tag (useful in CI). |
| `customTemplate` | User-defined | Maximum flexibility. |

### Recommended Setup for Development

```yaml
build:
  tagPolicy:
    inputDigest: {}    # Ensures the same source code → same tag (good for caching)
```

### Recommended Setup for CI/CD

```yaml
build:
  tagPolicy:
    gitCommit:
      variant: AbbrevCommitSha    # Short hash: abc1234
```

This makes it easy to trace a deployed image back to its Git commit.

---

## 3.4 — Build Modes: `local`, `cluster`, `googleCloudBuild`

Skaffold supports three distinct places to build images:

### `local` (Default)

```yaml
build:
  local:
    push: false          # Build on your machine, no push
    useBuildkit: true     # Enable BuildKit (default in modern Docker)
    concurrency: 0        # Build all artifacts in parallel (0 = unlimited)
```

Builds images on your local Docker daemon. When using Kind: 
- `push: false` tells Skaffold to use `kind load docker-image` instead of pushing to a registry
- This is the **fastest option** for local development

### `cluster`

```yaml
build:
  cluster:
    pullSecretName: my-secret
    namespace: build
```

Builds images **inside the Kubernetes cluster** using [Kaniko](https://github.com/GoogleContainerTools/kaniko). Useful when:
- You don't have Docker installed locally
- You're on a remote development cluster
- Company policy restricts local Docker usage

### `googleCloudBuild`

```yaml
build:
  googleCloudBuild:
    projectId: my-project
    timeout: 600s
```

Offloads builds to Google Cloud Build. Useful for:
- Powerful build machines (lots of CPU/RAM)
- Shared build cache across the team
- CI integration with GCP

**For this masterclass, we always use `local`.** It's the fastest for local development with Kind.

---

## 3.5 — Alternative Builders

Docker isn't the only way to build container images with Skaffold.

### Buildpacks

[Cloud Native Buildpacks](https://buildpacks.io/) detect your language and create an optimised image automatically — no Dockerfile needed:

```yaml
build:
  artifacts:
    - image: hello-app
      buildpacks:
        builder: gcr.io/buildpacks/builder:v1
        env:
          - GOOGLE_BUILDABLE=.    # Build the Go module in this directory
```

**Pros:** No Dockerfile to maintain, best practices built in.  
**Cons:** Less control, slower initial builds, larger images.

### Custom Builder

Run any script or command to build images:

```yaml
build:
  artifacts:
    - image: hello-app
      custom:
        buildCommand: ./build.sh
        dependencies:
          paths:
            - "**/*.go"
            - "go.mod"
```

**Use case:** When you need Bazel, Nix, or another build system that Skaffold doesn't natively support.

---

## 3.6 — Putting It All Together: Updated `skaffold.yaml`

Let's upgrade our `skaffold.yaml` from Chapter 2 with everything we've learned:

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
        noCache: false           # Use Docker's layer cache
      dependencies:
        paths:
          - "**/*.go"
          - "go.mod"
          - "go.sum"
          - "Dockerfile"
        ignore:
          - "**/*_test.go"
  tagPolicy:
    inputDigest: {}              # Content-based tags (great for caching)
  local:
    push: false                  # Direct load into Kind
    useBuildkit: true            # Enable BuildKit for cache mounts
    concurrency: 0               # Parallel builds if we add more artifacts

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

**Changes from Chapter 2:**
1. Added `dependencies` — only rebuild when `.go` files or dependency files change
2. Added `tagPolicy: inputDigest` — deterministic tags
3. Explicit `useBuildkit: true` — ensures cache mounts work
4. Added `concurrency: 0` — future-proofing for multi-artifact builds

---

## 3.7 — Updated Dockerfile (with BuildKit Cache Mounts)

Update your `project/Dockerfile` to match the optimised version:

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -o /app/server .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/server /server

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

---

## 3.8 — Build Performance Benchmarks

Here's what you should expect on a typical development machine (4-core CPU, 16 GB RAM):

| Build Scenario | Time (without cache mounts) | Time (with cache mounts) |
|---------------|---------------------------|-------------------------|
| Initial build (cold cache) | 25–40s | 25–40s |
| Rebuild after `go.mod` change | 15–25s | 3–8s |
| Rebuild after `.go` source change | 8–15s | 1–3s |
| No-op (no changes) | Skipped | Skipped |

**The key takeaway:** With cache mounts, your incremental rebuild time approaches that of running `go build` directly. The Docker overhead becomes negligible.

### Measuring Your Build Time

```bash
# Time a full rebuild
time skaffold build

# Enable Skaffold diagnostics for detailed timing
skaffold dev --verbosity=debug 2>&1 | grep -i "time\|duration\|took"
```

---

## 3.9 — What Could Go Wrong?

### ❌ `failed to solve: rpc error: mount not supported`

**Symptom:** Cache mounts fail during Docker build.  
**Cause:** BuildKit is not enabled or the `# syntax=docker/dockerfile:1` directive is missing.  
**Fix:**
```bash
# Ensure BuildKit is enabled
export DOCKER_BUILDKIT=1

# Or permanently in Docker's daemon.json:
# { "features": { "buildkit": true } }

# Make sure the FIRST line of your Dockerfile is:
# syntax=docker/dockerfile:1
```

### ❌ `standard_init_linux.go: exec user process caused: exec format error`

**Symptom:** Pod crashes immediately after deploy.  
**Cause:** The binary was compiled for the wrong architecture (e.g., you built an `arm64` binary on a Mac M1/M2, but Kind runs `amd64` nodes).  
**Fix:** Explicitly set `GOOS` and `GOARCH`:
```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...
```

Or use Docker's `--platform` flag:
```dockerfile
FROM --platform=linux/amd64 golang:1.23-alpine AS builder
```

### ❌ `image is too large (> 1 GB)`

**Symptom:** Slow image loading into Kind.  
**Cause:** You're probably using a single-stage Dockerfile with the `golang:*` image as the runtime base.  
**Fix:** Use multi-stage builds (as shown in this chapter). Your final image should be 5–15 MB, not 900 MB.

Quick check:
```bash
docker images hello-app
```

| Image | Size |
|-------|------|
| Single-stage with `golang:1.23` | ~900 MB |
| Multi-stage with `alpine` runtime | ~15 MB |
| Multi-stage with `distroless` | ~7 MB |
| Multi-stage with `scratch` | ~5 MB |

### ❌ `go: inconsistent vendoring` or `go.sum mismatch`

**Symptom:** Build fails during `go mod download`.  
**Cause:** Your `go.sum` is out of sync with `go.mod`.  
**Fix:**
```bash
go mod tidy
go mod verify
```

If using vendor mode, also run:
```bash
go mod vendor
```

### ❌ Skaffold rebuilds on every save, even non-Go files

**Symptom:** Editing `README.md` triggers a full image rebuild.  
**Cause:** No `dependencies` configuration — Skaffold watches all files.  
**Fix:** Add explicit dependency paths:
```yaml
build:
  artifacts:
    - image: hello-app
      dependencies:
        paths:
          - "**/*.go"
          - "go.mod"
          - "go.sum"
```

### ❌ Cache mounts don't seem to help

**Symptom:** Rebuilds are still slow despite cache mounts.  
**Fix:** Verify cache mounts are actually being used:
```bash
# Run a build with extra verbose output
DOCKER_BUILDKIT=1 docker build --progress=plain -t test .

# Look for lines like:
#  => importing cache manifest from local:...
#  => [cache mount /go/pkg/mod] cache hit
```

If you see "cache miss" on every build, your Docker daemon might be pruning caches. Check `docker system df` for cache usage.

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **Cache mounts** | Persist Go module and build caches across Docker builds. The biggest speed win. |
| **Multi-stage builds** | Compile in a large image, run in a tiny one. 900 MB → 7 MB. |
| **Tag policies** | `inputDigest` for dev, `gitCommit` for CI. Tags are how Skaffold connects builds to deploys. |
| **Dependencies** | Tell Skaffold which files to watch. Fewer watches = fewer unnecessary rebuilds. |
| **Build flags** | `CGO_ENABLED=0` + `-ldflags="-s -w"` = small, static, fast-starting binary. |

---

**← [Chapter 2: Hello, Skaffold](02-hello-skaffold.md)** | **[Chapter 4: The Deploy Pipeline →](04-deploy-pipeline.md)**
