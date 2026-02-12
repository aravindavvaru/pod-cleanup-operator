# pod-cleanup-operator

A Kubernetes operator that automatically cleans up pods based on configurable policies — by phase, age, namespace, and label selectors.

## Overview

The operator introduces a `PodCleanupPolicy` custom resource (CRD) to the `cleanup.k8s.io/v1` API group. Each policy defines *which* pods to target and *when* to clean them up. A built-in dry-run mode lets you validate selectors safely before enabling real deletion.

## Features

- **Phase-based cleanup** — target `Failed`, `Succeeded`, or any other pod phase
- **Age-based cleanup** — delete pods older than a configurable duration (`1h`, `24h`, `7d`, …)
- **Namespace scoping** — scan all namespaces or restrict with a label selector
- **Pod label filtering** — narrow cleanup to pods matching specific labels
- **Cron scheduling** — run cleanup on a cron schedule (e.g. `*/15 * * * *`)
- **Dry-run mode** — log what would be deleted without touching anything
- **Status reporting** — tracks last run time and cumulative/per-run pod counts

## Custom Resource: PodCleanupPolicy

```yaml
apiVersion: cleanup.k8s.io/v1
kind: PodCleanupPolicy
metadata:
  name: my-policy
spec:
  # Cron expression — how often to run. Omit to run on every reconcile.
  schedule: "*/15 * * * *"

  # Label-select target namespaces. Omit to scan all namespaces.
  namespaceSelector:
    matchLabels:
      environment: staging

  # Label-select pods to consider. Omit to consider all pods.
  podSelector:
    matchLabels:
      app: my-app

  # Pod phases eligible for deletion. Omit to match any phase.
  podStatuses:
    - Failed
    - Succeeded

  # Minimum age before a pod is eligible. Supports Go duration strings.
  maxAge: "1h"

  # Set to false to enable real deletion.
  dryRun: true
```

### Spec fields

| Field | Type | Default | Description |
|---|---|---|---|
| `schedule` | string | — | Cron expression for cleanup frequency |
| `namespaceSelector` | LabelSelector | all namespaces | Namespaces to scan |
| `podSelector` | LabelSelector | all pods | Pods to consider |
| `podStatuses` | []PodPhase | all phases | Pod phases eligible for deletion |
| `maxAge` | string (duration) | — | Minimum pod age to be eligible |
| `dryRun` | bool | `false` | Log-only mode; no pods are deleted |

### Status fields

| Field | Description |
|---|---|
| `lastRunTime` | Timestamp of the most recent cleanup run |
| `lastRunPodsDeleted` | Pods affected in the most recent run |
| `podsDeleted` | Cumulative pods deleted since creation |
| `conditions` | `Ready` condition with reason and message |

## Project Structure

```
pod-cleanup-operator/
├── api/v1/
│   ├── groupversion_info.go          # API group registration
│   ├── podcleanuppolicy_types.go     # CRD Go types
│   └── zz_generated.deepcopy.go     # Generated DeepCopy methods
├── cmd/
│   └── main.go                       # Operator entrypoint
├── config/
│   ├── crd/bases/                    # CRD manifest
│   ├── default/kustomization.yaml    # Default kustomize overlay
│   ├── manager/manager.yaml          # Deployment manifest
│   ├── rbac/                         # ServiceAccount, Role, RoleBinding
│   └── samples/                      # Example PodCleanupPolicy CRs
├── internal/controller/
│   └── podcleanuppolicy_controller.go # Reconciliation logic
├── Dockerfile
├── Makefile
└── go.mod
```

## Prerequisites

- Go 1.21+
- `kubectl` configured against a cluster
- Docker (for image builds)

## Getting Started

### 1. Install the CRD

```bash
make install
```

### 2. Run locally (out-of-cluster)

```bash
make run
```

### 3. Build and deploy to the cluster

```bash
# Build and push the image
make docker-build docker-push IMG=<registry>/pod-cleanup-operator:latest

# Deploy (sets image in the Deployment)
make deploy IMG=<registry>/pod-cleanup-operator:latest
```

### 4. Apply a sample policy

```bash
make sample
# or
kubectl apply -f config/samples/cleanup_v1_podcleanuppolicy.yaml
```

### 5. Check status

```bash
kubectl get podcleanuppolicies
kubectl describe podcleanuppolicy <name>
```

## Makefile targets

| Target | Description |
|---|---|
| `make build` | Compile the manager binary to `bin/manager` |
| `make run` | Run the controller locally against the current cluster |
| `make test` | Run tests |
| `make docker-build` | Build the container image |
| `make docker-push` | Push the container image |
| `make install` | Apply CRDs to the cluster |
| `make uninstall` | Remove CRDs from the cluster |
| `make deploy` | Deploy the operator via kustomize |
| `make undeploy` | Remove the operator from the cluster |
| `make sample` | Apply sample PodCleanupPolicy CRs |

## RBAC

The operator's ClusterRole grants:

- `get/list/watch/create/update/patch/delete` on `podcleanuppolicies`
- `get/list/watch/delete` on `pods`
- `get/list/watch` on `namespaces`
- `get/list/watch/create/update/patch/delete` on `leases` (leader election)

## Examples

### Clean up all Failed pods cluster-wide every hour

```yaml
apiVersion: cleanup.k8s.io/v1
kind: PodCleanupPolicy
metadata:
  name: cleanup-failed-pods
spec:
  schedule: "0 * * * *"
  podStatuses:
    - Failed
  maxAge: "30m"
  dryRun: false
```

### Clean up completed jobs in staging, every 5 minutes

```yaml
apiVersion: cleanup.k8s.io/v1
kind: PodCleanupPolicy
metadata:
  name: cleanup-staging-completed
spec:
  schedule: "*/5 * * * *"
  namespaceSelector:
    matchLabels:
      environment: staging
  podStatuses:
    - Succeeded
  maxAge: "10m"
  dryRun: false
```

### Dry-run — preview what would be deleted

```yaml
apiVersion: cleanup.k8s.io/v1
kind: PodCleanupPolicy
metadata:
  name: preview-cleanup
spec:
  schedule: "*/15 * * * *"
  podStatuses:
    - Failed
    - Succeeded
  maxAge: "1h"
  dryRun: true   # nothing will be deleted
```
