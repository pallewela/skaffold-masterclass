# Chapter 7 — Profiles, Environments & Multi-Config

> *"One config to rule them all, and in the profiles bind them."*

---

## What You'll Learn

- Skaffold **profiles** — environment-specific overrides within a single `skaffold.yaml`
- Profile **activation** methods: CLI flags, environment variables, kube-context
- Multi-module configurations with `requires`
- Sharing config across microservices
- Real-world patterns for `dev` / `staging` / `prod` setups

---

## 7.1 — Why Profiles?

In a real project, your development and production environments differ significantly:

| Concern | Development | Production |
|---------|------------|------------|
| Image base | `golang:*-alpine` (for hot reload) | `distroless` (minimal) |
| Build flags | `-gcflags="all=-N -l"` (debuggable) | `-ldflags="-s -w"` (stripped) |
| Replicas | 1 | 3+ |
| Image push | No (load into Kind) | Yes (push to registry) |
| Deploy tool | `kubectl` (simple) | `helm` (versioned) |
| Port forward | Yes | No |

Without profiles, you'd need separate `skaffold.yaml` files for each environment.  With profiles, one file handles everything.

---

## 7.2 — Profile Basics

A profile is a named set of overrides that Skaffold applies on top of the base configuration:

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: hello-app

# ── Base Configuration (shared defaults) ──
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

# ── Profiles (environment-specific overrides) ──
profiles:

  # Dev profile: hot reload with file sync
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

  # Staging profile: push to registry, deploy with Kustomize
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
          - k8s/overlays/staging

  # Production profile: push to registry, deploy with Helm
  - name: production
    build:
      local:
        push: true
      tagPolicy:
        gitCommit:
          variant: AbbrevCommitSha
    deploy:
      helm:
        releases:
          - name: hello-app
            chartPath: charts/hello-app
            valuesFiles:
              - charts/hello-app/values-prod.yaml
```

### How Profiles Override

Profiles use a **merge strategy**. They don't replace the entire base config; they merge at the stanza level:

```
Base config:          Profile override:       Result:
build:                build:                  build:
  artifacts:            artifacts:              artifacts:
    - image: hello      - image: hello          - image: hello
      docker:             docker:                 docker:
        dockerfile: X       dockerfile: Y           dockerfile: Y  ← overridden
  local:                                        local:
    push: false                                   push: false      ← preserved
```

Fields not specified in the profile are **inherited** from the base.

---

## 7.3 — Profile Activation

### Method 1: CLI Flag (`-p`)

```bash
skaffold dev -p dev           # Explicit activation
skaffold run -p staging
skaffold run -p production
```

### Method 2: Command-Based Activation

```yaml
profiles:
  - name: dev
    activation:
      - command: dev          # Auto-activates on `skaffold dev`
  - name: debug
    activation:
      - command: debug        # Auto-activates on `skaffold debug`
```

With this, `skaffold dev` automatically uses the `dev` profile without `-p`.

### Method 3: Kube-Context Activation

```yaml
profiles:
  - name: production
    activation:
      - kubeContext: gke_my-project_us-central1_prod-cluster
```

When your `kubectl` context points to the production cluster, the production profile activates automatically. **This is a safety feature** — it prevents you from accidentally deploying debug builds to production.

### Method 4: Environment Variable Activation

```yaml
profiles:
  - name: ci
    activation:
      - env: CI=true          # Activates when CI=true is set
```

GitHub Actions, GitLab CI, and most CI systems set `CI=true` automatically.

### Combining Multiple Activations

Activation rules within a single profile are **OR**'d — any match activates the profile:

```yaml
profiles:
  - name: production
    activation:
      - kubeContext: gke_*_prod*       # OR
      - env: DEPLOY_ENV=production     # either condition activates
```

---

## 7.4 — Profile Patches (Surgical Modifications)

If you only need to change a few fields, use `patches` instead of replacing entire stanzas:

```yaml
profiles:
  - name: high-memory
    patches:
      - op: replace
        path: /deploy/kubectl/manifests
        value:
          - k8s/base/*.yaml
          - k8s/overlays/high-memory/*.yaml
      - op: add
        path: /build/artifacts/0/docker/buildArgs
        value:
          GOMEMLIMIT: "512MiB"
```

Patches use [JSON Patch](https://jsonpatch.com/) syntax:

| Operation | Meaning |
|-----------|---------|
| `add` | Add a new field |
| `remove` | Remove a field |
| `replace` | Replace an existing field's value |
| `move` | Move a field |
| `copy` | Copy a field |

Patches are more precise but harder to read. Use them for minor tweaks; use full profile overrides for larger changes.

---

## 7.5 — Multi-Module Configurations with `requires`

As your project grows from one service to many, you can split your Skaffold config into multiple files and compose them:

### Project Structure

```
microservices/
├── api/
│   ├── main.go
│   ├── Dockerfile
│   ├── skaffold.yaml        # Config for the API service
│   └── k8s/
├── worker/
│   ├── main.go
│   ├── Dockerfile
│   ├── skaffold.yaml        # Config for the Worker service
│   └── k8s/
└── skaffold.yaml            # Root config that composes both
```

### Root `skaffold.yaml`

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: all-services

requires:
  - path: api
    activeProfiles:
      - name: dev
        activatedBy:
          - dev
  - path: worker
    activeProfiles:
      - name: dev
        activatedBy:
          - dev

profiles:
  - name: dev
    activation:
      - command: dev
```

### How `requires` Works

1. Skaffold reads the root `skaffold.yaml`
2. Discovers `requires` pointing to `api/skaffold.yaml` and `worker/skaffold.yaml`
3. Merges all three configs into a single pipeline
4. Profile activation cascades: activating `dev` on the root also activates `dev` on child modules (via `activeProfiles`)

### Run Everything

```bash
# From the root directory:
skaffold dev              # Builds and deploys both API and Worker
skaffold dev -m api       # Build and deploy only the API module
skaffold dev -m worker    # Build and deploy only the Worker module
```

The `-m` flag lets you selectively work on individual services when you don't need the full stack.

---

## 7.6 — Real-World Pattern: Complete Multi-Environment Setup

Here's a production-ready `skaffold.yaml` that ties together everything from Chapters 2–7:

```yaml
apiVersion: skaffold/v4beta11
kind: Config
metadata:
  name: hello-app

# ── Base: the shared foundation ──
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
  tagPolicy:
    inputDigest: {}
  local:
    push: false
    useBuildkit: true

deploy:
  kubectl:
    manifests:
      - k8s/base/*.yaml

portForward:
  - resourceType: service
    resourceName: hello-app
    port: 8080
    localPort: 8080

# ── Profiles ──
profiles:

  # Hot reload with file sync
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

  # Debug with Delve
  - name: debug
    build:
      artifacts:
        - image: hello-app
          docker:
            dockerfile: Dockerfile.debug
    activation:
      - command: debug

  # Staging: Kustomize + registry push
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
          - k8s/overlays/staging

  # Production: Helm + registry push
  - name: production
    build:
      local:
        push: true
      tagPolicy:
        gitCommit:
          variant: AbbrevCommitSha
    deploy:
      helm:
        releases:
          - name: hello-app
            chartPath: charts/hello-app
            valuesFiles:
              - charts/hello-app/values-prod.yaml
    activation:
      - kubeContext: gke_*_prod*
```

### Usage

```bash
skaffold dev                           # → dev profile (hot reload)
skaffold debug                         # → debug profile (Delve)
skaffold run                           # → base config (no profile)
skaffold run -p staging                # → staging profile
skaffold run -p production             # → production profile (or auto on prod context)
```

---

## 7.7 — What Could Go Wrong?

### ❌ Profile is not activating

**Symptom:** `skaffold dev` uses the base config instead of the `dev` profile.  
**Debug:**
```bash
skaffold diagnose
```

This shows which profiles are active and why. Check:
1. Does the `activation` stanza match? (command, env, kubeContext)
2. Is there a typo in the profile name?
3. Are you running the right Skaffold command?

### ❌ Profile overrides merge incorrectly

**Symptom:** Unexpected config after profile activation — some fields are there, some aren't.  
**Cause:** Profiles merge at the stanza level, not the field level. If you override `build.artifacts`, you must provide the **complete** artifacts list.  
**Fix:** When overriding `artifacts`, include all fields:
```yaml
profiles:
  - name: dev
    build:
      artifacts:
        - image: hello-app          # Must include image name
          docker:
            dockerfile: Dockerfile.dev
          sync:                      # And all additional config
            manual:
              - src: "**/*.go"
                dest: /app
```

### ❌ Circular dependency in multi-module config

**Symptom:**
```
Error: circular dependency detected: api → worker → api
```

**Cause:** Module A requires Module B, which requires Module A.  
**Fix:** Restructure your modules so dependencies form a **DAG** (Directed Acyclic Graph). Extract shared resources into a third module that both depend on.

### ❌ Wrong profile in production

**Symptom:** A debug or dev build ends up in production.  
**Prevention:** Use `kubeContext` activation as a safety net:
```yaml
profiles:
  - name: production
    activation:
      - kubeContext: gke_my-project_*_prod*    # Only on prod clusters
```

This ensures the production profile **only** activates when connected to the production cluster, regardless of what CLI flags you pass.

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **Profiles** | Environment-specific overrides in a single `skaffold.yaml` |
| **Activation** | CLI (`-p`), command auto-detect, kube-context, env vars |
| **Patches** | JSON Patch for surgical config modifications |
| **Multi-module** | `requires` composes multiple `skaffold.yaml` files into one pipeline |
| **Safety** | Use `kubeContext` activation to prevent deploying debug builds to prod |

---

**← [Chapter 6: Debugging](06-debugging.md)** | **[Chapter 8: Port Forwarding, Logging & Hooks →](08-advanced-devex.md)**
