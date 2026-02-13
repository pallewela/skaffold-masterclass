# Chapter 4 — The Deploy Pipeline: Manifests, Kustomize & Helm

> *"A Deployment without a Service is like a restaurant with no front door."*

---

## What You'll Learn

- How Skaffold's **deploy stage** works internally
- The three deployer types: **kubectl**, **Kustomize**, and **Helm**
- Deep-dive into `Deployment`, `Service`, `ConfigMap`, and `Ingress` resources
- How Skaffold's **image replacement** injects freshly-built tags into manifests
- Using **Kustomize overlays** for dev/staging/prod environments
- Introduction to **Helm charts** for templating
- How `statusCheck` works and why it matters

---

## 4.1 — How Skaffold Deploys

After building images, Skaffold hands the list of `image:tag` pairs to the **deploy stage**:

```
┌─────────────────────────────────────────────────┐
│                 DEPLOY PIPELINE                  │
│                                                  │
│  1. Read manifests / charts / overlays           │
│                                                  │
│  2. IMAGE REPLACEMENT                            │
│     Find image references matching artifact      │
│     names and replace with built image:tag       │
│                                                  │
│  3. Apply to cluster                             │
│     kubectl apply / helm upgrade / kustomize     │
│                                                  │
│  4. STATUS CHECK                                 │
│     Wait for Deployments/StatefulSets to         │
│     reach the desired state                      │
└─────────────────────────────────────────────────┘
```

### Image Replacement in Detail

This is one of Skaffold's most important features. In your `deployment.yaml`, you wrote:

```yaml
containers:
  - name: hello-app
    image: hello-app           # Just the name, no tag
```

During deploy, Skaffold scans all manifests for image references matching your artifact names. It replaces them with the fully-qualified, freshly-tagged image:

```yaml
containers:
  - name: hello-app
    image: hello-app:abc123@sha256:def456...    # Replaced by Skaffold
```

**This is why you never hardcode tags in your manifests.** Skaffold manages the tag lifecycle — you just use the image name.

---

## 4.2 — Deployer Type 1: `kubectl` (Raw Manifests)

This is what we've been using. It's the simplest approach:

```yaml
deploy:
  kubectl:
    manifests:
      - k8s/*.yaml
```

Skaffold reads your YAML files, performs image replacement, and runs the equivalent of:

```bash
kubectl apply -f <rendered-manifests> --namespace default
```

### When to Use `kubectl`

- **Small projects** with a handful of manifests
- **Learning Kubernetes** — you see exactly what's being applied
- **No templating needed** — values don't change between environments

### Limitations

- No templating (you can't parameterise values)
- No environment-specific overrides without duplicating files
- Manual management of resource ordering

---

## 4.3 — Kubernetes Resources Deep-Dive

Let's understand the core resources you'll use in every Skaffold project.

### Deployment

A Deployment declares the **desired state** of your application. Kubernetes continuously reconciles the actual state to match.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
  labels:
    app: hello-app
spec:
  replicas: 2                         # Run 2 identical Pods
  strategy:
    type: RollingUpdate               # Update Pods without downtime
    rollingUpdate:
      maxSurge: 1                     # At most 1 extra Pod during update
      maxUnavailable: 0               # Never have fewer than desired Pods
  selector:
    matchLabels:
      app: hello-app
  template:
    metadata:
      labels:
        app: hello-app
    spec:
      containers:
        - name: hello-app
          image: hello-app
          ports:
            - containerPort: 8080
          resources:                  # Resource limits (recommended)
            requests:
              cpu: "50m"              # 0.05 CPU cores
              memory: "32Mi"          # 32 MB RAM
            limits:
              cpu: "200m"             # 0.2 CPU cores
              memory: "128Mi"         # 128 MB RAM
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5
```

**New fields explained:**

- **`strategy.rollingUpdate`** — During a deploy, Kubernetes creates new Pods before killing old ones. `maxSurge: 1` means at most one extra Pod runs temporarily. `maxUnavailable: 0` means all old Pods stay running until new ones are ready.
- **`resources.requests`** — The *minimum* resources the Pod needs. The scheduler uses this to find a suitable node.
- **`resources.limits`** — The *maximum* resources the Pod can use. If it exceeds memory limits, it gets OOMKilled.

> **Go-specific note:** Go's garbage collector is aware of memory limits when you set `GOMEMLIMIT`. For production, consider setting `GOMEMLIMIT` to ~80% of your memory limit to prevent OOM kills:
> ```yaml
> env:
>   - name: GOMEMLIMIT
>     value: "100MiB"    # 80% of 128Mi limit
> ```

### ConfigMap

Store configuration data as key-value pairs:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hello-app-config
data:
  APP_ENV: "development"
  LOG_LEVEL: "debug"
  FEATURE_FLAGS: |
    {
      "enable_metrics": true,
      "enable_tracing": false
    }
```

**Use it in your Deployment:**

```yaml
spec:
  containers:
    - name: hello-app
      envFrom:
        - configMapRef:
            name: hello-app-config    # All keys become env vars
```

Let's add a ConfigMap to our project. Create `k8s/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hello-app-config
data:
  APP_ENV: "development"
  LOG_LEVEL: "debug"
```

And update `k8s/deployment.yaml` to reference it:

```yaml
spec:
  containers:
    - name: hello-app
      image: hello-app
      ports:
        - containerPort: 8080
      envFrom:
        - configMapRef:
            name: hello-app-config
      env:
        - name: PORT
          value: "8080"
```

### Ingress

Exposes HTTP routes from outside the cluster to Services inside:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hello-app-ingress
spec:
  rules:
    - host: hello.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: hello-app
                port:
                  number: 8080
```

> **Note:** Ingress requires an **Ingress Controller** (like NGINX Ingress) running in your cluster. For Kind, this requires extra setup. Since Skaffold's `portForward` gives us direct access, we won't use Ingress in local dev — but it's essential knowledge for production.

---

## 4.4 — Deployer Type 2: Kustomize

[Kustomize](https://kustomize.io/) lets you **overlay** changes on top of a base set of manifests — no templating language, just patches.

### Directory Structure

```
k8s/
├── base/                          # Shared across all environments
│   ├── kustomization.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml
└── overlays/
    ├── dev/                       # Development-specific patches
    │   ├── kustomization.yaml
    │   └── patch-replicas.yaml
    └── prod/                      # Production-specific patches
        ├── kustomization.yaml
        └── patch-replicas.yaml
```

### `k8s/base/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - deployment.yaml
  - service.yaml
  - configmap.yaml
```

### `k8s/overlays/dev/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

patches:
  - path: patch-replicas.yaml
```

### `k8s/overlays/dev/patch-replicas.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
spec:
  replicas: 1                     # Dev only needs 1 replica
```

### `k8s/overlays/prod/patch-replicas.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-app
spec:
  replicas: 3                     # Prod needs 3 replicas
```

### Skaffold with Kustomize

```yaml
deploy:
  kustomize:
    paths:
      - k8s/overlays/dev          # Default: use the dev overlay
```

Now `skaffold dev` applies the base manifests with the dev overlay patched on top. We'll use Skaffold **profiles** (Chapter 7) to switch between overlays.

---

## 4.5 — Deployer Type 3: Helm

[Helm](https://helm.sh/) is the Kubernetes "package manager." It uses Go templates for parameterisation.

### When to Use Helm Over Kustomize

| Feature | Kustomize | Helm |
|---------|-----------|------|
| **Approach** | Patch-based (overlay) | Template-based |
| **Learning curve** | Low | Medium |
| **Parameterisation** | Strategic merge patches | Go template variables |
| **Package sharing** | Not designed for it | First-class (chart repos) |
| **Rollback** | Manual (`kubectl`) | Built-in (`helm rollback`) |
| **Best for** | Internal config management | Shared, versioned deployments |

### Creating a Helm Chart

```bash
helm create charts/hello-app
```

This generates a full chart structure. For our project, we'll simplify it:

```
charts/hello-app/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.yaml
    └── service.yaml
```

### `charts/hello-app/values.yaml`

```yaml
replicaCount: 1

image:
  repository: hello-app
  tag: ""                          # Skaffold injects this

service:
  type: ClusterIP
  port: 8080

resources:
  limits:
    cpu: 200m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 32Mi

env:
  PORT: "8080"
  APP_ENV: "development"
```

### Skaffold with Helm

```yaml
deploy:
  helm:
    releases:
      - name: hello-app
        chartPath: charts/hello-app
        valuesFiles:
          - charts/hello-app/values.yaml
        setValues:
          image.repository: hello-app    # Matches the artifact name
```

**Image replacement with Helm** works differently than with kubectl. Skaffold sets the `image.tag` value to the built tag, and your Helm template uses `{{ .Values.image.repository }}:{{ .Values.image.tag }}`.

---

## 4.6 — Status Checks

After deploying, Skaffold doesn't just walk away. It runs **status checks** to verify your application is actually running:

```yaml
deploy:
  statusCheck: true                # Default: true
  statusCheckDeadlineSeconds: 120  # Max wait time (default: 120s)
  tolerateFailuresUntilDeadline: false
```

### What Status Check Does

1. Watches all Deployments/StatefulSets for the `Available` condition
2. Monitors Pod status for `Running`
3. Checks that readiness probes pass
4. Fails the pipeline if the deadline is exceeded

### What You'll See

**Success:**
```
Waiting for deployments to stabilize...
 - deployment/hello-app is ready. [1/1 deployment(s) available]
Deployments stabilized in 3.2s
```

**Failure:**
```
Waiting for deployments to stabilize...
 - deployment/hello-app: container hello-app is waiting to start:
   hello-app/hello-app-6f9b7c8d-x2k4m: container waiting for: ImagePullBackOff
 - deployment/hello-app failed. Error: deadline exceeded.
```

The failure message tells you exactly which Pod failed and why — a huge debugging time-saver.

---

## 4.7 — What Could Go Wrong?

### ❌ YAML Indentation Errors

**Symptom:**
```
error: error validating data: ValidationError(Deployment.spec):
  unknown field "container" in io.k8s.api.apps.v1.DeploymentSpec
```

**Cause:** YAML is indentation-sensitive. A misplaced space changes the structure entirely.  
**Fix:** Use a YAML linter:
```bash
# Install yamllint
pip install yamllint

# Lint all manifest files
yamllint k8s/
```

Or use `kubectl` dry-run:
```bash
kubectl apply -f k8s/ --dry-run=client
```

### ❌ Image Name Mismatch

**Symptom:** Skaffold deploys but the Pod pulls a random `hello-app:latest` from Docker Hub instead of your local build.  
**Cause:** The `image:` in your manifest doesn't exactly match the artifact name in `skaffold.yaml`.  
**Fix:** Ensure they match exactly:
```yaml
# skaffold.yaml
build:
  artifacts:
    - image: hello-app     # ← This name...

# deployment.yaml
containers:
  - image: hello-app       # ← ...must match this name
```

### ❌ `CrashLoopBackOff` — The Most Common Error

**Symptom:** Pod repeatedly crashes and restarts.  
**Debug systematic approach:**

```bash
# Step 1: Check the pod logs
kubectl logs -l app=hello-app --previous

# Step 2: Check pod events
kubectl describe pod -l app=hello-app

# Step 3: Check if the image is correct
kubectl get pod -l app=hello-app -o jsonpath='{.items[0].spec.containers[0].image}'

# Step 4: Run the image locally to verify
docker run --rm -p 8080:8080 hello-app:<tag>
```

**Common causes:**
1. Binary compiled for wrong OS/arch
2. Missing environment variable
3. Port already in use inside the container
4. Liveness probe failing (check the probe path/port)

### ❌ Kustomize `patch target not found`

**Symptom:**
```
couldn't find target for patch
```

**Cause:** The patch references a resource name or kind that doesn't exist in the base.  
**Fix:** Ensure the `metadata.name` and `kind` in your patch exactly match the base resource.

### ❌ Helm `template rendering failed`

**Symptom:**
```
Error: template: hello-app/templates/deployment.yaml:15:
  function "Values" not defined
```

**Cause:** Helm uses Go template syntax (`{{ .Values.foo }}`). Missing the dot before `Values` is a common mistake.  
**Fix:** Always use `{{ .Values.key }}` (with the dot prefix).

---

## Summary

| Deployer | Complexity | Templating | Best For |
|----------|-----------|------------|----------|
| `kubectl` | Low | None | Learning, small projects |
| `kustomize` | Medium | Patch-based overlays | Multi-environment config |
| `helm` | Medium-High | Go templates | Shared charts, version control |

| Concept | Key Takeaway |
|---------|-------------|
| **Image replacement** | Skaffold auto-injects built tags into manifests. Never hardcode tags. |
| **ConfigMaps** | Store non-secret config; inject as env vars with `envFrom`. |
| **Resource limits** | Always set `requests` and `limits`. Go respects `GOMEMLIMIT`. |
| **Status checks** | Skaffold waits for pods to be ready — fast failure detection. |

---

**← [Chapter 3: The Build Pipeline](03-build-pipeline.md)** | **[Chapter 5: The Dev Loop →](05-dev-loop.md)**
