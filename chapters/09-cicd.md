# Chapter 9 — CI/CD Integration & The Outer Loop

> *"The Inner Loop makes developers fast. The Outer Loop makes deployments reliable."*

---

## What You'll Learn

- The **Outer Loop** — how Skaffold fits into CI/CD pipelines
- `skaffold build` — build and push images without deploying
- `skaffold deploy` — deploy pre-built images
- `skaffold render` — generate manifests for GitOps workflows
- Integration with **GitHub Actions**, **GitLab CI**, and **Google Cloud Build**
- Artifact caching and layer reuse strategies in CI

---

## 9.1 — Inner Loop vs. Outer Loop (Revisited)

In Chapter 1, we introduced these concepts. Now let's operationalise them:

```
┌──────────────────────────────────────────────────────────────────┐
│                          DEVELOPER                                │
│                                                                   │
│   INNER LOOP (local machine)          OUTER LOOP (CI/CD)         │
│   ┌───────────────────────┐           ┌────────────────────────┐ │
│   │ skaffold dev           │    git    │ skaffold build         │ │
│   │  → build               │───push──►│  → build               │ │
│   │  → deploy (local K8s)  │          │  → tag                 │ │
│   │  → watch & iterate     │          │  → push to registry    │ │
│   └───────────────────────┘           │                        │ │
│                                       │ skaffold deploy        │ │
│                                       │  → deploy to staging   │ │
│                                       │  → status check        │ │
│                                       │                        │ │
│                                       │ skaffold render        │ │
│                                       │  → generate manifests  │ │
│                                       │  → commit to GitOps    │ │
│                                       └────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

**The key insight:** Skaffold uses the **same** `skaffold.yaml` for both loops. Your local dev config drives CI/CD — no separate pipeline definitions that drift out of sync.

---

## 9.2 — Skaffold CI/CD Commands

Skaffold separates its pipeline into discrete commands for CI/CD:

| Command | What It Does | CI Use Case |
|---------|-------------|-------------|
| `skaffold build` | Build & push images | Build stage in CI |
| `skaffold test` | Run container-structure tests | Test stage in CI |
| `skaffold deploy` | Deploy pre-built images | Deploy stage in CI |
| `skaffold render` | Generate rendered manifests | GitOps (Argo CD, Flux) |
| `skaffold run` | Build + deploy (all-in-one) | Simple CI pipelines |
| `skaffold verify` | Run verification tests after deploy | Post-deploy testing |

### `skaffold build`

Builds all artifacts and pushes them to a container registry:

```bash
# Build, tag, and push — output the built tags to a file
skaffold build \
  --file-output=build.json \
  --default-repo=ghcr.io/myorg/myapp \
  --tag=v1.2.3
```

Output `build.json`:

```json
{
  "builds": [
    {
      "imageName": "hello-app",
      "tag": "ghcr.io/myorg/myapp/hello-app:v1.2.3@sha256:abc123..."
    }
  ]
}
```

### `skaffold deploy`

Deploys using pre-built images (from `build.json`):

```bash
skaffold deploy \
  --build-artifacts=build.json \
  --profile=staging
```

This is the **decoupled build/deploy** pattern — build once, deploy many times.

### `skaffold render`

Generates the final Kubernetes manifests without applying them:

```bash
skaffold render \
  --build-artifacts=build.json \
  --output=rendered-manifests.yaml \
  --profile=production
```

This is essential for **GitOps** — you render the manifests, commit them to a Git repo, and let Argo CD or Flux apply them.

---

## 9.3 — Tag Policies for CI

In local development, the tag doesn't matter much. In CI, it matters a lot:

### `gitCommit` — Most Common for CI

```yaml
build:
  tagPolicy:
    gitCommit:
      variant: AbbrevCommitSha    # Short hash: abc1234
      # variant: CommitSha        # Full hash: abc1234def5678...
      # variant: AbbrevTreeSha    # Based on file tree content
      # variant: TreeSha          # Full tree hash
```

**Why:** Every commit produces a unique, traceable tag. You can always find which code a running pod is built from.

### `sha256` — Content-Based

```yaml
build:
  tagPolicy:
    sha256: {}
```

Tag is the SHA256 of the image content. Useful when you want idempotent builds — identical source always produces identical tags.

### `dateTime` — Simple Sequential

```yaml
build:
  tagPolicy:
    dateTime:
      format: "2006-01-02_15-04-05"    # Go time format
      timezone: "UTC"
```

### `envTemplate` — Custom Format

```yaml
build:
  tagPolicy:
    envTemplate:
      template: "{{.GIT_TAG}}-{{.BUILD_NUMBER}}"
```

Use with environment variables set by your CI system:

```bash
export GIT_TAG=$(git describe --tags)
export BUILD_NUMBER=$CI_PIPELINE_IID
skaffold build
```

### `customTemplate` — Full Control

```yaml
build:
  tagPolicy:
    customTemplate:
      template: "{{.SHA}}-{{.DATE}}"
      components:
        - name: SHA
          gitCommit:
            variant: AbbrevCommitSha
        - name: DATE
          dateTime:
            format: "20060102"
```

Produces tags like: `abc1234-20260213`

---

## 9.4 — Container Registry Setup

CI pipelines push images to a registry. Here's how to configure common registries:

### Docker Hub

```yaml
build:
  local:
    push: true
  artifacts:
    - image: myorg/hello-app    # Docker Hub org/repo
```

```bash
# In CI, authenticate before building:
echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin
```

### GitHub Container Registry (ghcr.io)

```yaml
build:
  local:
    push: true
  artifacts:
    - image: hello-app    # Skaffold prepends --default-repo
```

```bash
# Authenticate
echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_ACTOR" --password-stdin

# Build with registry prefix
skaffold build --default-repo=ghcr.io/myorg
```

### Google Artifact Registry

```bash
# Authenticate with GCP
gcloud auth configure-docker us-central1-docker.pkg.dev

# Build with GCP registry
skaffold build --default-repo=us-central1-docker.pkg.dev/my-project/my-repo
```

### The `--default-repo` Pattern

Instead of hardcoding registry URLs in `skaffold.yaml`, use `--default-repo`:

```bash
# Local dev: no registry (load into Kind)
skaffold dev

# CI staging: push to staging registry
skaffold build --default-repo=ghcr.io/myorg/staging

# CI production: push to production registry
skaffold build --default-repo=ghcr.io/myorg/production
```

Your `skaffold.yaml` stays environment-agnostic — the registry is a runtime parameter.

---

## 9.5 — GitHub Actions Integration

### Basic Pipeline

`.github/workflows/ci.yaml`:

```yaml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  SKAFFOLD_DEFAULT_REPO: ghcr.io/${{ github.repository }}
  SKAFFOLD_CACHE_ARTIFACTS: true

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Install Skaffold
        run: |
          curl -Lo skaffold https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-amd64
          chmod +x skaffold
          sudo mv skaffold /usr/local/bin/

      - name: Build & Push
        run: |
          skaffold build \
            --file-output=build.json \
            --default-repo=${{ env.SKAFFOLD_DEFAULT_REPO }}

      - name: Upload build artifacts
        uses: actions/upload-artifact@v4
        with:
          name: skaffold-build
          path: build.json

  deploy-staging:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download build artifacts
        uses: actions/download-artifact@v4
        with:
          name: skaffold-build

      - name: Install Skaffold
        run: |
          curl -Lo skaffold https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-amd64
          chmod +x skaffold
          sudo mv skaffold /usr/local/bin/

      - name: Configure kubectl
        uses: azure/k8s-set-context@v4
        with:
          kubeconfig: ${{ secrets.KUBE_CONFIG_STAGING }}

      - name: Deploy to Staging
        run: |
          skaffold deploy \
            --build-artifacts=build.json \
            --profile=staging
```

### Key Patterns

1. **Separate build and deploy jobs** — build once, deploy to multiple environments
2. **Pass `build.json` between jobs** — the deploy job knows exactly which images to use
3. **Use `--default-repo`** — keep your `skaffold.yaml` environment-agnostic
4. **Cache Docker layers** — use Buildx cache for faster builds

---

## 9.6 — GitLab CI Integration

`.gitlab-ci.yml`:

```yaml
stages:
  - build
  - deploy

variables:
  SKAFFOLD_DEFAULT_REPO: $CI_REGISTRY_IMAGE

build:
  stage: build
  image: docker:24-dind
  services:
    - docker:24-dind
  before_script:
    - echo "$CI_REGISTRY_PASSWORD" | docker login -u "$CI_REGISTRY_USER" --password-stdin $CI_REGISTRY
    - |
      curl -Lo skaffold https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-amd64
      chmod +x skaffold && mv skaffold /usr/local/bin/
  script:
    - skaffold build --file-output=build.json
  artifacts:
    paths:
      - build.json
    expire_in: 1 hour

deploy-staging:
  stage: deploy
  image: bitnami/kubectl:latest
  needs: [build]
  before_script:
    - |
      curl -Lo skaffold https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-amd64
      chmod +x skaffold && mv skaffold /usr/local/bin/
  script:
    - skaffold deploy --build-artifacts=build.json --profile=staging
  environment:
    name: staging
  only:
    - main
```

---

## 9.7 — Google Cloud Build Integration

`cloudbuild.yaml`:

```yaml
steps:
  # Build and push with Skaffold
  - name: 'gcr.io/k8s-skaffold/skaffold'
    args:
      - 'build'
      - '--file-output=/workspace/build.json'
      - '--default-repo=us-central1-docker.pkg.dev/$PROJECT_ID/my-repo'

  # Deploy to GKE
  - name: 'gcr.io/k8s-skaffold/skaffold'
    args:
      - 'deploy'
      - '--build-artifacts=/workspace/build.json'
      - '--profile=production'
    env:
      - 'CLOUDSDK_COMPUTE_ZONE=us-central1-a'
      - 'CLOUDSDK_CONTAINER_CLUSTER=prod-cluster'

options:
  machineType: 'E2_HIGHCPU_8'    # Faster builds
```

Google Cloud Build has **first-class Skaffold support** — the `gcr.io/k8s-skaffold/skaffold` image is maintained by the Skaffold team.

---

## 9.8 — GitOps with `skaffold render`

GitOps replaces imperative `kubectl apply` with a **declarative Git-based workflow**:

```
┌──────────┐     ┌──────────┐     ┌──────────────┐     ┌─────────────┐
│ Developer │────►│ CI Build │────►│ skaffold     │────►│ GitOps Repo │
│ git push  │     │ + push   │     │ render       │     │ (manifests) │
└──────────┘     └──────────┘     │ + commit     │     └──────┬──────┘
                                  └──────────────┘            │
                                                              │ sync
                                                    ┌─────────▼──────────┐
                                                    │ Argo CD / Flux     │
                                                    │ → kubectl apply    │
                                                    │ → cluster state    │
                                                    └────────────────────┘
```

### Render Step

```bash
# After building:
skaffold render \
  --build-artifacts=build.json \
  --profile=production \
  --output=deploy/rendered.yaml
```

This produces a fully-rendered manifest with actual image tags:

```yaml
# deploy/rendered.yaml (generated)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
spec:
  template:
    spec:
      containers:
        - name: hello-app
          image: ghcr.io/myorg/hello-app:abc1234@sha256:def456...  # ← Real tag
```

### Commit to GitOps Repository

```bash
cd /path/to/gitops-repo
cp /tmp/rendered.yaml apps/hello-app/
git add .
git commit -m "Deploy hello-app: abc1234"
git push
```

Argo CD or Flux detects the change and applies it to the cluster.

### GitHub Actions GitOps Workflow

```yaml
  render-and-commit:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'

    steps:
      - name: Checkout app repo
        uses: actions/checkout@v4

      - name: Download build artifacts
        uses: actions/download-artifact@v4
        with:
          name: skaffold-build

      - name: Install Skaffold
        run: |
          curl -Lo skaffold https://storage.googleapis.com/skaffold/releases/latest/skaffold-linux-amd64
          chmod +x skaffold && sudo mv skaffold /usr/local/bin/

      - name: Render manifests
        run: |
          skaffold render \
            --build-artifacts=build.json \
            --profile=production \
            --output=rendered-manifests.yaml

      - name: Checkout GitOps repo
        uses: actions/checkout@v4
        with:
          repository: myorg/gitops-config
          token: ${{ secrets.GITOPS_PAT }}
          path: gitops

      - name: Update manifests
        run: |
          cp rendered-manifests.yaml gitops/apps/hello-app/manifests.yaml
          cd gitops
          git config user.name "CI Bot"
          git config user.email "ci@example.com"
          git add .
          git commit -m "Deploy hello-app: ${{ github.sha }}"
          git push
```

---

## 9.9 — CI Caching Strategies

Building Docker images in CI is slow without caching. Here are strategies to speed it up:

### Strategy 1: Docker Layer Caching with BuildKit

```yaml
# In your GitHub Actions workflow:
- name: Set up Docker Buildx
  uses: docker/setup-buildx-action@v3

- name: Build with cache
  run: |
    skaffold build \
      --default-repo=${{ env.SKAFFOLD_DEFAULT_REPO }}
  env:
    DOCKER_BUILDKIT: 1
```

### Strategy 2: Registry-Based Cache

Use the Docker `--cache-from` / `--cache-to` flags via Skaffold:

```yaml
build:
  artifacts:
    - image: hello-app
      docker:
        dockerfile: Dockerfile
        cacheFrom:
          - ghcr.io/myorg/hello-app:cache
```

After building, the layer cache is stored in the registry and reused in subsequent builds.

### Strategy 3: Go Module Cache

Mount the Go module cache in CI:

```yaml
# GitHub Actions
- name: Cache Go modules
  uses: actions/cache@v4
  with:
    path: |
      ~/go/pkg/mod
      ~/.cache/go-build
    key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
    restore-keys: |
      ${{ runner.os }}-go-
```

With BuildKit cache mounts in the Dockerfile (Chapter 3), this cached directory is available during the Docker build.

---

## 9.10 — `skaffold verify` — Post-Deploy Testing

After deploying, you can run verification tests:

```yaml
verify:
  - name: integration-tests
    container:
      name: integration
      image: curlimages/curl:latest
      command: ["sh"]
      args:
        - "-c"
        - |
          echo "Waiting for service..."
          sleep 10
          curl -sf http://hello-app:8080/health || exit 1
          echo "Service is healthy!"
```

Run it:

```bash
skaffold verify --build-artifacts=build.json
```

This creates a Kubernetes Job that runs inside the cluster, with access to cluster-internal DNS — perfect for testing service-to-service communication.

---

## 9.11 — What Could Go Wrong?

### ❌ Registry authentication failure in CI

**Symptom:**
```
denied: permission denied
```

**Cause:** Missing or expired credentials.
**Fix by CI platform:**

| Platform | Fix |
|----------|-----|
| GitHub Actions | Ensure `packages: write` permission and `docker/login-action` |
| GitLab CI | Use `$CI_REGISTRY_PASSWORD` (auto-set) |
| Cloud Build | Ensure the Cloud Build service account has Artifact Registry Writer role |

### ❌ `build.json` not found in deploy job

**Symptom:**
```
error reading build artifacts: open build.json: no such file or directory
```

**Cause:** The `build.json` artifact wasn't passed between CI jobs.
**Fix:** Use your CI platform's artifact mechanism:
- GitHub Actions: `upload-artifact` / `download-artifact`
- GitLab CI: `artifacts.paths`
- Cloud Build: `/workspace/` is shared across steps

### ❌ Image pull error after deploy

**Symptom:**
```
ErrImagePull: rpc error: pull access denied
```

**Cause:** The cluster doesn't have credentials to pull from the registry.
**Fix:** Create an `imagePullSecret`:
```bash
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=<user> \
  --docker-password=<token>
```

Reference in your deployment:
```yaml
spec:
  imagePullSecrets:
    - name: ghcr-secret
```

### ❌ Manifest drift — rendered manifests don't match cluster state

**Symptom:** Argo CD shows "OutOfSync" even after deploying.
**Cause:** The rendered manifests include Skaffold-specific labels (`skaffold.dev/run-id`) that change on every render.
**Fix:** Use `--set-value-from` or strip Skaffold labels in your render pipeline:
```bash
skaffold render --build-artifacts=build.json | \
  yq eval 'del(.metadata.labels["skaffold.dev/run-id"])' - > rendered.yaml
```

### ❌ CI builds are slow (5+ minutes)

**Cause:** No Docker layer caching, no Go module caching.
**Fix (checklist):**
1. ✅ Enable Docker BuildKit (`DOCKER_BUILDKIT=1`)
2. ✅ Use BuildKit cache mounts in your Dockerfile (Chapter 3)
3. ✅ Cache Go modules in CI (`actions/cache`)
4. ✅ Use `cacheFrom` with a registry-based cache
5. ✅ Use a fast CI runner (larger machine type)

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **`skaffold build`** | Build and push images; output `build.json` for downstream stages |
| **`skaffold deploy`** | Deploy using pre-built images from `build.json` |
| **`skaffold render`** | Generate manifests for GitOps (Argo CD / Flux) |
| **`skaffold verify`** | Run post-deploy integration tests inside the cluster |
| **Tag policies** | `gitCommit` for CI traceability; `envTemplate` for custom formats |
| **`--default-repo`** | Keep `skaffold.yaml` registry-agnostic; set registry at runtime |
| **Caching** | BuildKit + Go module cache = fast CI builds |

---

**← [Chapter 8: Port Forwarding, Logging & Hooks](08-advanced-devex.md)** | **[Chapter 10: Capstone →](10-capstone.md)**
