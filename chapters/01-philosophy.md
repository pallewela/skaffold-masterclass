# Chapter 1 — The Philosophy: What Is Skaffold & Why It Exists

> *"The best tool is one that disappears into your workflow."*  
> — Skaffold design philosophy

---

## What You'll Learn

- What the **Inner Development Loop** and **Outer Development Loop** are — and why you should care
- Where Skaffold sits in the cloud-native tooling landscape
- The four DevEx scenarios Skaffold addresses
- A high-level tour of every stanza in `skaffold.yaml`

---

## 1.1 — The Problem: Death by a Thousand `kubectl apply`s

Imagine you're a Go developer building a REST API. Without Skaffold, your local Kubernetes development loop looks like this:

```
1.  Edit main.go
2.  docker build -t myapp:v42 .
3.  docker tag myapp:v42 registry.example.com/myapp:v42
4.  docker push registry.example.com/myapp:v42
5.  Edit deployment.yaml to update the image tag to v42
6.  kubectl apply -f deployment.yaml
7.  kubectl get pods                          # wait for it...
8.  kubectl port-forward svc/myapp 8080:8080
9.  curl http://localhost:8080/health
10. Realise you have a typo. Go to step 1.
```

**Ten steps. Every. Single. Change.**

This is the pain that Skaffold eliminates. It watches your source code, rebuilds the image, re-deploys to your cluster, sets up port forwarding, and tails your logs — all with a single running command:

```bash
skaffold dev
```

But before we run anything, let's understand the *mental model* behind Skaffold.

---

## 1.2 — The Inner Loop vs. The Outer Loop

These two terms define the lifecycle of code from your editor to production. Understanding them is the single most important concept for using Skaffold effectively.

### The Inner Loop (Where You Live)

The **Inner Loop** is the tight cycle a developer repeats hundreds of times a day:

```
Code → Build → Test → Run → Observe → Code → ...
```

**Characteristics:**
- Happens on your **local machine** (or a dev cluster)
- Must be **fast** — ideally under 10 seconds per iteration
- Requires **real-time feedback** — logs, errors, hot reload
- Is **personal** — only you see the results

Traditional Go development has a blazing-fast inner loop: `go run main.go` takes milliseconds. But the moment you introduce Kubernetes — containers, manifests, clusters — the loop balloons to minutes. Skaffold's primary mission is to **restore that sub-10-second inner loop for Kubernetes development**.

### The Outer Loop (Where CI/CD Lives)

The **Outer Loop** is everything that happens after you push code:

```
Git Push → CI Build → Run Tests → Security Scan → Deploy to Staging → Integration Tests → Deploy to Prod
```

**Characteristics:**
- Happens in a **CI/CD system** (GitHub Actions, GitLab CI, Cloud Build, etc.)
- **Slower** — minutes to hours is acceptable
- Must be **reproducible** — the same commit should produce the same artifact
- Is **shared** — the whole team depends on it

Here's the key insight: **Skaffold bridges both loops using the same configuration file.** Your `skaffold.yaml` drives `skaffold dev` on your laptop *and* `skaffold build` + `skaffold deploy` in your CI pipeline. One file, two contexts. This is by design.

```
┌─────────────────────────────────────────────────────┐
│                    skaffold.yaml                     │
│                                                     │
│  ┌──────────────────┐     ┌──────────────────────┐  │
│  │   INNER LOOP     │     │    OUTER LOOP        │  │
│  │                  │     │                      │  │
│  │  skaffold dev    │     │  skaffold build      │  │
│  │  skaffold debug  │     │  skaffold deploy     │  │
│  │  skaffold run    │     │  skaffold render     │  │
│  │                  │     │                      │  │
│  │  ► Local cluster │     │  ► CI/CD pipeline    │  │
│  │  ► Fast feedback │     │  ► Reproducible      │  │
│  │  ► Developer-only│     │  ► Team-shared       │  │
│  └──────────────────┘     └──────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

---

## 1.3 — Where Skaffold Sits in the Landscape

Skaffold isn't the only tool trying to improve the Kubernetes developer experience. Here's an honest comparison:

| Feature | **Skaffold** | Tilt | DevSpace | Garden |
|---------|-------------|------|----------|--------|
| **Maintainer** | Google (open-source) | Docker, Inc. | Loft Labs | Garden.io |
| **Config format** | YAML (`skaffold.yaml`) | Starlark (Python-like) | YAML (`devspace.yaml`) | YAML (`garden.yml`) |
| **Build support** | Docker, Buildpacks, Jib, Bazel, Custom | Docker, Custom | Docker, Custom | Docker, Custom |
| **Deploy support** | kubectl, Kustomize, Helm | kubectl, Helm | kubectl, Helm | Kubernetes, Terraform |
| **Debug support** | Built-in (Go/Delve, Java, Node, Python) | Via extensions | Limited | Limited |
| **CI/CD story** | First-class (`skaffold build`, `render`) | Primarily local dev | Primarily local dev | Has CI/CD features |
| **Complexity** | Low | Medium | Medium | High |
| **Web UI** | No (CLI-first) | Yes (Tilt UI) | No | Yes (Dashboard) |

### Why choose Skaffold?

1. **Simplicity** — Pure YAML, no scripting language to learn
2. **Composability** — Mix and match build and deploy tools
3. **CI/CD bridge** — Same config works in your pipeline
4. **Debugger integration** — `skaffold debug` instruments your Go binary with Delve automatically
5. **Google-backed, CNCF-adjacent** — Large community, active development

> **A note about Tilt and DevSpace:** These are excellent tools. If you've used DevSpace before (as in our [multi-service debugging project](chapters/06-debugging.md)), you'll recognise many shared concepts. Skaffold's differentiator is its laser focus on being a **thin orchestration layer** rather than a full platform.

---

## 1.4 — The Four DevEx Scenarios

Skaffold is designed to handle four distinct development scenarios. Understanding which one you're in will determine which Skaffold commands and configuration you use.

### Scenario 1: Local Development (`skaffold dev`)

**You're at your desk, iterating rapidly.**

- Skaffold watches your files
- On change: rebuild → redeploy → port-forward → tail logs
- On `Ctrl+C`: cleans up all deployed resources (unless you use `--cleanup=false`)

This is the scenario 90% of developers will use 90% of the time. It's the *raison d'être* of Skaffold.

```bash
skaffold dev --port-forward
```

### Scenario 2: One-Shot Deployment (`skaffold run`)

**You want to deploy once and walk away.**

- Builds and deploys, but does **not** watch for file changes
- Does **not** clean up on exit
- Useful for deploying to a shared dev cluster or running integration tests

```bash
skaffold run --tail
```

### Scenario 3: Debugging (`skaffold debug`)

**You need to step through your Go code running inside a pod.**

- Like `skaffold dev`, but it:
  - Rewrites your container command to start Delve
  - Exposes the debug port
  - Disables Go compiler optimisations (`-gcflags='all=-N -l'`)
- Works with VS Code, GoLand, and any DAP-compatible editor

```bash
skaffold debug --port-forward
```

### Scenario 4: CI/CD Pipeline (`skaffold build` + `skaffold deploy`)

**You're in a CI system, not at your desk.**

- `skaffold build` — Build and push images, output the built tags
- `skaffold deploy` — Deploy pre-built images to a cluster
- `skaffold render` — Output fully-rendered Kubernetes manifests (for GitOps)

```bash
# In CI:
skaffold build --file-output=artifacts.json
skaffold deploy --build-artifacts=artifacts.json
```

---

## 1.5 — Anatomy of `skaffold.yaml`

Every Skaffold project revolves around a single configuration file: `skaffold.yaml`. Let's walk through its top-level stanzas. Don't worry about memorising this — we'll explore each one in depth in later chapters.

```yaml
# skaffold.yaml — Annotated Overview
apiVersion: skaffold/v4beta11    # Schema version (always use the latest)
kind: Config                     # Always "Config"
metadata:
  name: my-go-app                # Human-readable project name

build:                           # HOW to build your container images
  artifacts:                     # List of images to build
    - image: my-go-app           # Image name (Skaffold manages the tag)
      docker:                    # Builder type: docker, buildpacks, jib, custom
        dockerfile: Dockerfile
  local:                         # Build locally (vs. in-cluster or on Google Cloud Build)
    push: false                  # Don't push to a registry (Kind loads images directly)

deploy:                          # HOW to deploy to Kubernetes
  kubectl:                       # Deployer type: kubectl, kustomize, helm
    manifests:
      - k8s/*.yaml               # Path to your Kubernetes manifests

portForward:                     # Automatically forward ports to localhost
  - resourceType: service
    resourceName: my-go-app
    port: 8080
    localPort: 8080

profiles:                        # Environment-specific overrides
  - name: production
    build:
      local:
        push: true               # Push images when deploying to prod
    deploy:
      helm:                      # Use Helm for production deploys
        releases:
          - name: my-go-app
            chartPath: charts/my-go-app
```

### Stanza Breakdown

| Stanza | Purpose | Chapter |
|--------|---------|---------|
| `build` | Defines what images to build and how | [Chapter 3](03-build-pipeline.md) |
| `deploy` | Defines how to deploy to Kubernetes | [Chapter 4](04-deploy-pipeline.md) |
| `portForward` | Automatically forward pod/service ports to localhost | [Chapter 8](08-advanced-devex.md) |
| `profiles` | Environment-specific configuration overrides | [Chapter 7](07-profiles.md) |
| `test` | Run tests (container-structure-tests, custom) after build | [Chapter 3](03-build-pipeline.md) |
| `verify` | Run verification tasks after deploy | [Chapter 9](09-cicd.md) |
| `customActions` | Run arbitrary scripts at defined lifecycle points | [Chapter 8](08-advanced-devex.md) |

---

## 1.6 — A Brief Kubernetes Primer (For the K8s-Curious)

If you're new to Kubernetes, here are the five concepts you need to follow this masterclass. We'll explain each in more detail when we first use it.

### Pod
The smallest deployable unit in Kubernetes. A Pod wraps one or more containers. Think of it as a "logical host" for your Go binary.

### Deployment
Manages a set of identical Pods. It ensures the desired number of replicas are running and handles rolling updates. **You almost never create Pods directly** — you create a Deployment and let Kubernetes manage the Pods.

### Service
A stable network endpoint that routes traffic to your Pods. Pods are ephemeral (they can die and restart with new IPs), but a Service provides a fixed DNS name and port.

### ConfigMap / Secret
Key-value stores for configuration data. ConfigMaps hold non-sensitive data (feature flags, URLs); Secrets hold sensitive data (API keys, passwords). Both can be injected into Pods as environment variables or mounted as files.

### Namespace
A virtual partition of your cluster. It's like a folder for your Kubernetes resources. By default, everything goes into the `default` namespace.

```
┌─────────────────── Cluster ────────────────────┐
│                                                 │
│  ┌──── Namespace: default ───────────────────┐  │
│  │                                           │  │
│  │  ┌─── Deployment: my-go-app ───────────┐  │  │
│  │  │                                     │  │  │
│  │  │  ┌── Pod ──┐    ┌── Pod ──┐        │  │  │
│  │  │  │  Go     │    │  Go     │        │  │  │
│  │  │  │  binary │    │  binary │        │  │  │
│  │  │  └─────────┘    └─────────┘        │  │  │
│  │  └─────────────────────────────────────┘  │  │
│  │                                           │  │
│  │  ┌─── Service: my-go-app ──────────────┐  │  │
│  │  │  ClusterIP: 10.96.42.1:8080         │  │  │
│  │  │  Routes to ──► Deployment Pods      │  │  │
│  │  └─────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

---

## 1.7 — How Skaffold Actually Works (The Pipeline)

When you run `skaffold dev`, Skaffold executes a **pipeline** with these stages:

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  WATCH   │───►│  BUILD   │───►│  TEST    │───►│  DEPLOY  │
│          │    │          │    │          │    │          │
│ Detect   │    │ Docker   │    │ Run      │    │ kubectl  │
│ file     │    │ build /  │    │ container│    │ apply /  │
│ changes  │    │ Buildpack│    │ tests    │    │ Helm     │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
      ▲                                              │
      │              ┌──────────┐                    │
      │              │  STATUS  │                    │
      └──────────────│  CHECK   │◄───────────────────┘
                     │          │
                     │ Wait for │
                     │ pods to  │
                     │ be ready │
                     └──────────┘
```

1. **Watch** — Skaffold monitors your project directory for file changes (uses `fsnotify` under the hood)
2. **Build** — Triggers a Docker build (or Buildpack, or custom builder), tags the image, and loads it into your cluster
3. **Test** — Runs any configured tests (container structure tests, custom test commands)
4. **Deploy** — Applies your Kubernetes manifests with the new image tag
5. **Status Check** — Waits for Pods to reach the `Running` state and pass readiness probes
6. **Loop** — Returns to watching for the next file change

For `skaffold run`, it executes steps 2–5 once and exits. For `skaffold debug`, it adds debugger instrumentation between Build and Deploy.

---

## 1.8 — What Could Go Wrong?

### ❌ "I don't understand when to use `skaffold dev` vs `skaffold run`"

| Command | Watches files? | Cleans up on exit? | Use case |
|---------|---------------|-------------------|----------|
| `skaffold dev` | ✅ Yes | ✅ Yes (by default) | Active development |
| `skaffold run` | ❌ No | ❌ No | One-shot deploy, CI |
| `skaffold debug` | ✅ Yes | ✅ Yes | Step-through debugging |

**Rule of thumb:** If you're editing code, use `dev`. If you're deploying, use `run`.

### ❌ "Isn't Skaffold just Docker Compose for Kubernetes?"

No. Docker Compose orchestrates containers on a **single Docker host**. Skaffold orchestrates the **build-test-deploy pipeline** targeting a **Kubernetes cluster**. They operate at different abstraction levels:

- **Docker Compose:** "Run these containers together on my machine"
- **Skaffold:** "Build these images, apply these K8s manifests, and keep everything in sync as I code"

You can actually use Docker Compose *with* Skaffold (via custom deployers), but they serve fundamentally different purposes.

### ❌ "Do I need a cloud Kubernetes cluster to use Skaffold?"

Absolutely not. Skaffold works beautifully with local clusters:

- **Kind** (Kubernetes IN Docker) — Runs K8s nodes as Docker containers. Lightweight and fast.
- **Minikube** — Runs K8s in a VM or container. More features, slightly heavier.
- **K3d** — Runs K3s (lightweight K8s) in Docker. Very fast startup.

This masterclass uses **Kind** because it's the simplest to set up and works identically on Linux, macOS, and Windows (via WSL2).

### ❌ "Will Skaffold slow down my Go development?"

The initial build will be slower than `go run main.go` because you're building a Docker image. But with proper caching (covered in [Chapter 3](03-build-pipeline.md)):

- **Incremental rebuilds** take 2–5 seconds
- **File sync** (no rebuild) takes < 1 second
- **Compared to manual steps:** Skaffold saves 30–60 seconds per iteration

Over a day of development, that's **hours** saved.

---

## Summary

| Concept | Key Takeaway |
|---------|-------------|
| **Inner Loop** | The fast code-build-test cycle on your machine. Skaffold optimises this. |
| **Outer Loop** | The CI/CD pipeline. Skaffold's config works here too. |
| **`skaffold.yaml`** | The single source of truth for your build and deploy pipeline. |
| **Skaffold Pipeline** | Watch → Build → Test → Deploy → Status Check → Loop |
| **Local Clusters** | Kind, Minikube, or K3d — no cloud required. |

---

**Next Chapter:** [Hello, Skaffold: Your First Go Service on Kubernetes →](02-hello-skaffold.md)
