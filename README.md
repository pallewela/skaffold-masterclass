# Skaffold Masterclass for Go Developers

> A definitive, multi-chapter deep-dive into Skaffold for Go development on local Kubernetes clusters. From conceptual foundations to advanced DevEx workflows.

---

## Who Is This For?

You know Go. You may have heard of Kubernetes. You're tired of the `docker build → docker push → kubectl apply` treadmill and want a tool that automates your **Inner Development Loop**. This masterclass will make you feel in *total control* of Skaffold.

## Prerequisites

| Tool | Minimum Version | Purpose |
|------|----------------|---------|
| [Go](https://go.dev/dl/) | 1.21+ | Application language |
| [Docker](https://docs.docker.com/get-docker/) | 24+ | Container builds |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | 1.28+ | Kubernetes CLI |
| [Kind](https://kind.sigs.k8s.io/) | 0.20+ | Local Kubernetes cluster |
| [Skaffold](https://skaffold.dev/docs/install/) | 2.10+ | The star of the show |

## Curriculum

### Part I — Conceptual Foundations & Setup

| # | Chapter | Summary |
|---|---------|---------|
| 1 | [The Philosophy: What Is Skaffold & Why It Exists](chapters/01-philosophy.md) | Inner Loop vs. Outer Loop, the CNCF landscape, Skaffold's architecture |
| 2 | [Hello, Skaffold: Your First Go Service on Kubernetes](chapters/02-hello-skaffold.md) | Bootstrap the sample project, first `skaffold dev` run |

### Part II — Deep-Dive Mechanics

| # | Chapter | Summary |
|---|---------|---------|
| 3 | [The Build Pipeline: Docker, BuildKit & Go Module Caching](chapters/03-build-pipeline.md) | Tag policies, multi-stage builds, Go cache mounts |
| 4 | [The Deploy Pipeline: Manifests, Kustomize & Helm](chapters/04-deploy-pipeline.md) | `kubectl`, Kustomize overlays, Helm charts, image replacement |
| 5 | [The Dev Loop: File Sync, Hot Reload & Rebuild Triggers](chapters/05-dev-loop.md) | File watching, sync modes, CompileDaemon for Go |
| 6 | [Debugging Go in Kubernetes with Skaffold](chapters/06-debugging.md) | `skaffold debug`, Delve, VS Code / GoLand integration |

### Part III — Advanced DevEx & Production Readiness

| # | Chapter | Summary |
|---|---------|---------|
| 7 | [Profiles, Environments & Multi-Config](chapters/07-profiles.md) | Profile activation, multi-module configs, microservice sharing |
| 8 | [Port Forwarding, Logging, Lifecycle Hooks & Custom Actions](chapters/08-advanced-devex.md) | `portForward`, log tailing, hooks, `statusCheck` |
| 9 | [CI/CD Integration & The Outer Loop](chapters/09-cicd.md) | `skaffold build/deploy/render`, GitHub Actions, GitOps |
| 10 | [Capstone: Production-Grade Patterns & Troubleshooting Clinic](chapters/10-capstone.md) | Multi-service architecture, performance tuning, reference table |

## The Sample Project

The [`project/`](project/) directory contains the evolving Go application you'll build through the chapters. Each chapter shows you the exact changes to make; the `project/` directory holds the **final state**.

```
project/
├── main.go            # Go HTTP service
├── go.mod
├── Dockerfile         # Multi-stage, BuildKit-optimised
├── skaffold.yaml      # Fully-featured Skaffold config
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

## How to Use This Masterclass

1. **Read sequentially** — the chapters build on each other.
2. **Type the code** — don't copy-paste. Muscle memory matters.
3. **Break things** — every chapter has a *"What could go wrong?"* section. Try triggering those errors intentionally.
4. **Experiment** — modify the sample project beyond what's asked.

---

*Built with ☕ and `skaffold dev` running in the background.*
