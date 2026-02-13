# Chapter 10 — Capstone: Production-Grade Patterns & Troubleshooting Clinic

> *"The best way to learn Skaffold is to build something real — and then watch it break."*

---

## What You'll Learn

- Evolving the sample project into a **multi-service architecture** (API + Worker)
- Production-grade `skaffold.yaml` with all the techniques from Chapters 1–9
- **Performance tuning** — reducing build times, image sizes, and deploy latency
- A comprehensive **troubleshooting reference table** for every common Skaffold error
- Where to go next: Telepresence, Service Meshes, Operators, and beyond

---

## 10.1 — The Final Architecture

Let's evolve our simple Hello World service into a realistic architecture:

```
                        ┌──────────────────────┐
                        │       Ingress         │
                        │    (hello.local)      │
                        └──────────┬───────────┘
                                   │
                        ┌──────────▼───────────┐
                        │     API Service       │
                        │   (Go HTTP server)    │
                        │                       │
                        │  GET /                │
                        │  GET /health          │
                        │  POST /jobs           │
                        │  GET /jobs/:id        │
                        └──────────┬───────────┘
                                   │
                          enqueue  │  via Redis
                                   │
                        ┌──────────▼───────────┐
                        │    Worker Service     │
                        │   (Go background)     │
                        │                       │
                        │  Dequeue & process    │
                        │  jobs from Redis      │
                        └──────────────────────┘
                                   │
                        ┌──────────▼───────────┐
                        │       Redis           │
                        │   (in-cluster)        │
                        └──────────────────────┘
```

### Project Structure (Final State)

```
skaffold-tut/project/
├── main.go                    # API service (enhanced)
├── worker/
│   └── main.go                # Worker service
├── go.mod
├── go.sum
├── Dockerfile                 # Production build (API)
├── Dockerfile.dev             # Dev build with CompileDaemon
├── Dockerfile.debug           # Debug build with Delve
├── worker/Dockerfile          # Production build (Worker)
├── skaffold.yaml              # Multi-artifact config
├── k8s/
│   ├── base/
│   │   ├── kustomization.yaml
│   │   ├── api-deployment.yaml
│   │   ├── api-service.yaml
│   │   ├── worker-deployment.yaml
│   │   ├── redis-deployment.yaml
│   │   ├── redis-service.yaml
│   │   └── configmap.yaml
│   └── overlays/
│       ├── dev/
│       │   ├── kustomization.yaml
│       │   └── patch-replicas.yaml
│       └── prod/
│           ├── kustomization.yaml
│           └── patch-replicas.yaml
└── scripts/
    ├── seed.sh
    └── migrate.sh
```

---

## 10.2 — Multi-Service `skaffold.yaml`

Here's the complete, production-grade configuration:

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: hello-app

# ── Build ──
build:
  artifacts:
    # API Service
    - image: hello-app-api
      context: .
      docker:
        dockerfile: Dockerfile
        buildArgs:
          SERVICE: api
      dependencies:
        paths:
          - "*.go"
          - "go.mod"
          - "go.sum"
        ignore:
          - "**/*_test.go"
          - "worker/**"

    # Worker Service
    - image: hello-app-worker
      context: .
      docker:
        dockerfile: worker/Dockerfile
      dependencies:
        paths:
          - "worker/**/*.go"
          - "go.mod"
          - "go.sum"
        ignore:
          - "**/*_test.go"

  tagPolicy:
    inputDigest: {}
  local:
    push: false
    useBuildkit: true
    concurrency: 2              # Build both images in parallel

# ── Deploy ──
deploy:
  statusCheck: true
  statusCheckDeadlineSeconds: 120
  kubectl:
    manifests:
      - k8s/base/*.yaml
    hooks:
      after:
        - host:
            command: ["sh", "-c", "echo '✅ All services deployed!'"]

# ── Port Forwards ──
portForward:
  - resourceType: service
    resourceName: hello-app-api
    port: 8080
    localPort: 8080
  - resourceType: service
    resourceName: redis
    port: 6379
    localPort: 6379

# ── Profiles ──
profiles:

  # Development: hot reload with file sync
  - name: dev
    build:
      artifacts:
        - image: hello-app-api
          context: .
          docker:
            dockerfile: Dockerfile.dev
          sync:
            manual:
              - src: "*.go"
                dest: /app
        - image: hello-app-worker
          context: .
          docker:
            dockerfile: worker/Dockerfile
    deploy:
      kustomize:
        paths:
          - k8s/overlays/dev
    activation:
      - command: dev

  # Debug: Delve-instrumented builds
  - name: debug
    build:
      artifacts:
        - image: hello-app-api
          context: .
          docker:
            dockerfile: Dockerfile.debug
        - image: hello-app-worker
          context: .
          docker:
            dockerfile: worker/Dockerfile
    activation:
      - command: debug

  # Staging: push to registry + Kustomize
  - name: staging
    build:
      local:
        push: true
      tagPolicy:
        gitCommit:
          variant: AbbrevCommitSha
    deploy:
      kustomize:
        paths:
          - k8s/overlays/dev       # Staging uses dev-like config

  # Production: push to registry + Kustomize prod overlay
  - name: production
    build:
      local:
        push: true
      tagPolicy:
        gitCommit:
          variant: AbbrevCommitSha
    deploy:
      kustomize:
        paths:
          - k8s/overlays/prod
    activation:
      - kubeContext: gke_*_prod*

# ── Verification ──
verify:
  - name: health-check
    container:
      name: curl-check
      image: curlimages/curl:latest
      command: ["sh"]
      args:
        - "-c"
        - |
          echo "Waiting for API..."
          sleep 10
          curl -sf http://hello-app-api:8080/health || exit 1
          echo "API is healthy!"
```

### Usage Cheat Sheet

```bash
# Local development (hot reload)
skaffold dev

# Debug with Delve
skaffold debug

# One-off deploy (base config)
skaffold run

# Staging
skaffold run -p staging --default-repo=ghcr.io/myorg

# Production (auto-activates on prod kube-context)
skaffold run -p production --default-repo=ghcr.io/myorg

# CI: build → deploy (decoupled)
skaffold build --file-output=build.json --default-repo=ghcr.io/myorg
skaffold deploy --build-artifacts=build.json -p staging

# GitOps: render manifests
skaffold render --build-artifacts=build.json -p production --output=rendered.yaml

# Post-deploy verification
skaffold verify --build-artifacts=build.json
```

---

## 10.3 — Performance Tuning

### Build Time Optimisations

| Technique | Impact | Chapter |
|-----------|--------|---------|
| Multi-stage Dockerfile | Smaller images, faster deploys | 2, 3 |
| BuildKit cache mounts (`--mount=type=cache`) | 2–5x faster Go builds | 3 |
| `dependencies.paths` + `ignore` | Only rebuild when relevant files change | 7 |
| `build.local.concurrency: 2` | Build multiple images in parallel | 10 |
| `tagPolicy: inputDigest` | Skip rebuild if inputs haven't changed | 3, 9 |
| File sync + CompileDaemon | Skip Docker build entirely | 5 |

### Image Size Optimisations

| Technique | Typical Size | Notes |
|-----------|-------------|-------|
| `golang:alpine` (single-stage) | ~300MB | Includes entire Go toolchain |
| Multi-stage with `alpine` runtime | ~15MB | Minimal runtime |
| Multi-stage with `distroless` | ~8MB | No shell, maximum security |
| Multi-stage with `scratch` | ~5MB | Absolute minimum; no debugging tools |

**Our Dockerfile produces ~8MB images** using `distroless`.

### Optimised Production Dockerfile

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build with all optimisations
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w -X main.version=$(git describe --tags 2>/dev/null || echo dev)" \
      -trimpath \
      -o /app/server .

# Runtime: distroless for security
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
```

**New flags explained:**

| Flag | Effect |
|------|--------|
| `-trimpath` | Remove file system paths from the binary (reproducible builds) |
| `-X main.version=...` | Embed the git tag as a version string |
| `GOARCH=amd64` | Explicit architecture (prevents cross-compilation surprises) |

### Deploy Latency Optimisations

| Technique | Impact |
|-----------|--------|
| Kind image preloading (`kind load docker-image`) | Avoid pulling from registry |
| `push: false` for local dev | Skip registry push entirely |
| `statusCheckDeadlineSeconds: 60` | Fail fast when things go wrong |
| Small images (distroless) | Faster image loading into nodes |

---

## 10.4 — Comprehensive Troubleshooting Reference

Here's every common Skaffold error, organised by category.

### Build Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `Cannot connect to the Docker daemon` | Docker isn't running | Start Docker: `sudo systemctl start docker` |
| `COPY failed: file not found` | File path in Dockerfile is wrong | Check paths relative to the build context |
| `go: cannot find main module` | `go.mod` missing from build context | Ensure `COPY go.mod ./` in Dockerfile |
| `go build: no Go files in /app` | Source not copied | Add `COPY . .` after dependency caching steps |
| `exec format error` | Binary built for wrong architecture | Set `GOOS=linux GOARCH=amd64` in build |
| `BuildKit not supported` | Docker version too old | Upgrade Docker or set `DOCKER_BUILDKIT=0` |
| `Error building image: tag not found` | Invalid tag policy | Check `tagPolicy` in `skaffold.yaml` |

### Deploy Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `CrashLoopBackOff` | Container exits immediately | Check logs: `kubectl logs -l app=<name> --previous` |
| `ImagePullBackOff` | Can't pull the image | Verify image name matches artifact; check `push` setting |
| `OOMKilled` | Container exceeded memory limit | Increase `resources.limits.memory`; set `GOMEMLIMIT` |
| `CreateContainerConfigError` | Bad env var or mount | Check `kubectl describe pod <name>` |
| `Deployment exceeded progress deadline` | Status check timeout | Increase `statusCheckDeadlineSeconds` |
| `Invalid YAML` | Indentation / syntax error | Run `kubectl apply --dry-run=client -f <file>` |
| `Service has no endpoints` | Selector doesn't match pods | Verify label selectors match between Service and Deployment |

### File Sync Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `Changed files are not syncable` | File doesn't match sync rule | Check glob pattern in `sync.manual[].src` |
| `Sync failed: container not running` | Pod crashed or restarting | Fix the underlying crash, Skaffold will retry |
| `inotify watch limit reached` | Too many files watched | Increase `fs.inotify.max_user_watches` |
| Synced file doesn't take effect | CompileDaemon not running | Check `Dockerfile.dev` ENTRYPOINT |

### Debug Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `Unverified breakpoint` | Path mapping wrong | Check `substitutePath` in `launch.json` |
| `could not attach to pid` | Binary is optimised | Build with `-gcflags="all=-N -l"` |
| `port 56268 already in use` | Stale debug session | Kill process: `lsof -i :56268` → `kill <PID>` |
| Debugger connects but hangs | Binary stripped (`-s -w`) | Remove `-ldflags="-s -w"` from debug builds |

### Profile / Config Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `Profile not found` | Typo in profile name | Check `profiles[].name` in `skaffold.yaml` |
| `Circular dependency` | Module A requires B, B requires A | Restructure into a DAG |
| Profile doesn't activate | Activation condition not met | Run `skaffold diagnose` to check |
| Unexpected config after profile | Merge semantics surprise | Override complete stanzas, not partial fields |

### CI/CD Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `denied: permission denied` | Registry auth failure | Login before build; check permissions |
| `build.json: no such file` | Artifact not passed between jobs | Use CI artifact upload/download |
| `ErrImagePull` in cluster | Cluster can't reach registry | Create `imagePullSecrets` |
| `skaffold: command not found` | Skaffold not installed in CI | Add installation step |

---

## 10.5 — Production Checklist

Before moving your Skaffold setup to production, verify:

### Dockerfile

- [ ] Multi-stage build (builder → runtime)
- [ ] `CGO_ENABLED=0` for static binary
- [ ] `distroless` or `scratch` base for runtime
- [ ] BuildKit cache mounts for Go modules and build cache
- [ ] Non-root user (`USER nonroot:nonroot`)
- [ ] `-ldflags="-s -w"` to strip debug symbols in production
- [ ] `-trimpath` for reproducible builds
- [ ] Explicit `GOOS` and `GOARCH`

### Kubernetes Manifests

- [ ] Resource `requests` and `limits` set
- [ ] Liveness probe configured
- [ ] Readiness probe configured
- [ ] `GOMEMLIMIT` env var set (~80% of memory limit)
- [ ] `imagePullPolicy: IfNotPresent` (or `Always` for mutable tags)
- [ ] Security context: `runAsNonRoot: true`
- [ ] Labels for observability: `app`, `version`, `team`

### Skaffold Config

- [ ] `push: false` for local development
- [ ] `push: true` in CI/staging/production profiles
- [ ] `tagPolicy: gitCommit` for CI builds (traceability)
- [ ] `statusCheck: true` with appropriate deadline
- [ ] Profiles for each environment (`dev`, `staging`, `production`)
- [ ] `kubeContext` activation as a safety net for production
- [ ] `dependencies.paths` and `ignore` to minimise rebuilds
- [ ] `--default-repo` used in CI (not hardcoded in config)

### CI/CD

- [ ] Separate build and deploy stages
- [ ] `build.json` artifact passed between stages
- [ ] Docker layer caching enabled
- [ ] Go module caching enabled
- [ ] Registry credentials configured
- [ ] `imagePullSecrets` in target clusters
- [ ] Post-deploy verification (`skaffold verify`)

---

## 10.6 — Where to Go Next

### Telepresence — Bridge Your Local Machine into the Cluster

[Telepresence](https://www.telepresence.io/) lets you run a single service locally while routing cluster traffic to it. Combined with Skaffold:

```
Cluster:   [API] → [Worker] → [Redis]
                ↕ (intercepted)
Local:     [API running locally with Telepresence]
```

Use when:
- You need to debug a service that depends on many in-cluster resources
- Full `skaffold dev` is too slow because you have 10+ services
- You want IDE debugging without `skaffold debug` overhead

### Service Meshes (Istio, Linkerd)

When your microservices grow, a service mesh provides:
- Mutual TLS between services
- Traffic management (canary deploys, circuit breakers)
- Observability (distributed tracing)

Skaffold deploys mesh sidecars transparently — your `skaffold.yaml` doesn't change, but the mesh injects proxies alongside your containers.

### Kubernetes Operators

For stateful workloads (databases, message queues), operators manage the lifecycle:

```yaml
# Example: deploy a PostgreSQL cluster with an operator
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: hello-db
spec:
  instances: 3
  storage:
    size: 10Gi
```

Skaffold deploys operator CRDs and instances alongside your application.

### Skaffold Plugins and Integrations

- **Cloud Code** — Google's IDE extension (VS Code / IntelliJ) with built-in Skaffold support
- **Skaffold + ko** — Build Go images without Docker (directly from Go source)
- **Skaffold + Buildpacks** — Auto-detect language and build without a Dockerfile

---

## 10.7 — Final Thoughts

You've come a long way:

| Chapter | What You Learned |
|---------|-----------------|
| 1 | Skaffold's philosophy — Inner Loop automation |
| 2 | Your first Go project running on Kubernetes |
| 3 | Build pipeline — multi-stage Docker, BuildKit caching |
| 4 | Deploy pipeline — kubectl, Kustomize, Helm |
| 5 | Dev loop — file sync, hot reload, trigger modes |
| 6 | Debugging — Delve, VS Code, GoLand integration |
| 7 | Profiles — multi-environment config in one file |
| 8 | DevEx — port forwarding, logging, hooks, actions |
| 9 | CI/CD — outer loop, GitOps, registry integration |
| 10 | Production — multi-service, performance, troubleshooting |

**The mental model to take away:**

Skaffold is a **pipeline orchestrator** for your development workflow. It doesn't replace Docker, Kubernetes, Helm, or your CI system — it **connects** them with a single configuration file that works identically on your laptop and in CI.

```
┌──────────────────────────────────────────────┐
│              skaffold.yaml                    │
│                                              │
│   One file. Every environment. Every stage.  │
│                                              │
│   Local dev  →  skaffold dev                 │
│   Debugging  →  skaffold debug               │
│   CI build   →  skaffold build               │
│   CI deploy  →  skaffold deploy              │
│   GitOps     →  skaffold render              │
│   Testing    →  skaffold verify              │
└──────────────────────────────────────────────┘
```

Now go build something great. And when it breaks, you know where to look.

---

**← [Chapter 9: CI/CD Integration](09-cicd.md)** | **[Back to README →](../README.md)**
