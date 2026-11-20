# meepctl — Troubleshooting & Operations Guide

> **Purpose:** Quick-reference guide for MEC Sandbox operators to diagnose failures and restart individual components using `meepctl`, without bringing down the entire platform.

---

## Table of Contents

- [Platform Architecture (Deployment Layers)](#platform-architecture-deployment-layers)
- [meepctl Command Reference](#meepctl-command-reference)
  - [build](#build)
  - [dockerize](#dockerize)
  - [deploy](#deploy)
  - [delete](#delete)
- [Recent meepctl Enhancements](#recent-meepctl-enhancements)
- [Decision Tree: What Changed → What to Do](#decision-tree-what-changed--what-to-do)
- [Troubleshooting & Restart Procedures](#troubleshooting--restart-procedures)
  - [Restarting a Dependency (dep) App](#restarting-a-dependency-dep-app)
  - [Restarting a Core App](#restarting-a-core-app)
  - [Restarting a Sandbox MEC Service (API App)](#restarting-a-sandbox-mec-service-api-app)
  - [Full Platform Restart](#full-platform-restart)
- [Common Failure Scenarios](#common-failure-scenarios)

---

## Platform Architecture (Deployment Layers)

MEC Sandbox has three deployment layers, each managed differently:

| Layer | Description | Deployed via | Examples |
|-------|-------------|--------------|----------|
| **dep** (Dependencies) | Third-party infrastructure services | `meepctl deploy dep all` | meep-couchdb, meep-grafana, meep-influxdb, meep-ingress, meep-postgis, meep-prometheus, meep-redis, meep-thanos |
| **core** (Core Platform) | AdvantEDGE platform microservices | `meepctl deploy core all` | meep-auth-svc, meep-mon-engine, meep-platform-ctrl, meep-virt-engine, meep-webhook, meep-sandbox-api |
| **sandbox** (MEC API Services) | MEC service API implementations (per-user sandbox) | Managed by **meep-virt-engine** at runtime | meep-loc-serv (MEC013), meep-rnis (MEC012), meep-tm (MEC015), meep-ams (MEC021), meep-dai (MEC016), meep-wais (MEC028), meep-vis (MEC030), meep-iot (MEC033), meep-sss (MEC046), meep-federation (MEC040), meep-app-enablement (MEC011) |

> **Key insight:** Sandbox MEC services are **not** deployed directly via `meepctl deploy`. They are packaged inside the `meep-virt-engine` Docker image (as Helm charts in the `docker-data` section) and deployed dynamically when a user creates a sandbox session.

---

## meepctl Command Reference

### build

Build Go/JS application binaries.

```bash
# Build all apps
meepctl build all

# Build a single app
meepctl build meep-ams
meepctl build meep-platform-ctrl

# Build without linting
meepctl build meep-ams --nolint
```

### dockerize

Create Docker images and push to the local registry.

```bash
# Dockerize all apps
meepctl dockerize all

# Dockerize a single app
meepctl dockerize meep-ams
meepctl dockerize meep-virt-engine

# Dockerize without Docker cache (clean build)
meepctl dockerize meep-ams --no-cache

# Dockerize with a custom registry
meepctl dockerize meep-ams --registry my.registry.com
```

### deploy

Deploy Helm charts to the K8s cluster.

```bash
# Deploy all dependencies
meepctl deploy dep all

# Deploy a single dependency
meepctl deploy dep meep-grafana
meepctl deploy dep meep-couchdb

# Deploy all core apps
meepctl deploy core all

# Deploy a single core app
meepctl deploy core meep-platform-ctrl

# Force re-deploy (delete + 5s wait + deploy)
meepctl deploy dep meep-grafana --force
meepctl deploy core meep-virt-engine --force

# Deploy with a specific image tag / registry
meepctl deploy core all --registry my.registry.com --tag v1.0.0
meepctl deploy core meep-auth-svc --tag v2.0.0
```

### delete

Remove Helm releases from the K8s cluster.

```bash
# Delete all dependencies
meepctl delete dep

# Delete a single dependency
meepctl delete dep meep-grafana

# Delete all core apps
meepctl delete core

# Delete a single core app
meepctl delete core meep-platform-ctrl
```

---

## Recent meepctl Enhancements

The following improvements have been made to `meepctl` to simplify operations:

| Feature                           | Description                                                                                                                  |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| **Individual core deploy/delete** | `meepctl deploy core <app>` and `meepctl delete core <app>` — deploy or delete a single core app instead of the entire group |
| **Individual dep deploy/delete**  | `meepctl deploy dep <app>` and `meepctl delete dep <app>` — deploy or delete a single dependency app                         |
| **Force-deploy cooldown**         | 5-second wait between delete and re-deploy when using `--force`, allowing Kubernetes to clean up resources                   |
| **`--no-cache` dockerize flag**   | `meepctl dockerize <app> --no-cache` — build Docker images from scratch without using cached layers                          |
| **Dockerize filename fix**        | Safely handles filenames with spaces in dockerize checksum calculation                                                       |
| **Permissive workdir creation**   | Uses `0777` permissions for `~/.meep` directories so Kubernetes `fsGroup` can manage ownership properly                      |
| **Lint v2 compatibility**         | Fixed `golangci-lint` v2 compatibility issues in meepctl source code                                                         |


---

## Decision Tree: What Changed → What to Do

Use this decision tree to determine the correct restart procedure after making code or configuration changes:

### Scenario 1: Changed Go source code of a MEC API service

> **Example:** Updated `go-apps/meep-ams/` (MEC021 Application Mobility)

Sandbox MEC services are bundled inside `meep-virt-engine`. However, each service has its **own** Docker image. So:

```bash
# 1. Build the changed app
meepctl build meep-ams

# 2. Dockerize ONLY the changed app (creates new Docker image)
meepctl dockerize meep-ams

# 3. Restart: delete any active user sandbox that uses this service,
#    then recreate the sandbox from the MEC Sandbox UI.
#    The new sandbox will pull the updated Docker image.
```

> **No need** to rebuild or redeploy `meep-virt-engine` unless you also changed its Helm chart.

### Scenario 2: Changed a Helm chart of a MEC API service

> **Example:** Updated `charts/meep-ams/` (chart templates, values, etc.)

The Helm charts for sandbox services are **packaged inside** the `meep-virt-engine` Docker image. So:

```bash
# 1. Re-dockerize meep-virt-engine (picks up the updated chart from docker-data)
meepctl dockerize meep-virt-engine

# 2. Force re-deploy meep-virt-engine to load the new charts
meepctl deploy core meep-virt-engine --force

# 3. Recreate any active user sandbox from the MEC Sandbox UI
```

### Scenario 3: Changed Go source code of a core app

> **Example:** Updated `go-apps/meep-platform-ctrl/`

```bash
# 1. Build the app
meepctl build meep-platform-ctrl

# 2. Dockerize the app
meepctl dockerize meep-platform-ctrl

# 3. Force re-deploy the specific core app
meepctl deploy core meep-platform-ctrl --force
```

### Scenario 4: Changed a core app's Helm chart

> **Example:** Updated `charts/meep-platform-ctrl/`

```bash
# No build/dockerize needed — just re-deploy
meepctl deploy core meep-platform-ctrl --force
```

### Scenario 5: Changed a dependency's Helm chart or values

> **Example:** Updated `charts/grafana/` or user values in `~/.meep/user/values/meep-grafana.yaml`

```bash
# Force re-deploy the specific dependency
meepctl deploy dep meep-grafana --force
```

### Summary Table

| What changed | build | dockerize | deploy |
|-------------|-------|-----------|--------|
| Go code of a MEC API service (e.g. `go-apps/meep-ams/`) | `meepctl build meep-ams` | `meepctl dockerize meep-ams` | Recreate sandbox from UI |
| Helm chart of a MEC API service (e.g. `charts/meep-ams/`) | — | `meepctl dockerize meep-virt-engine` | `meepctl deploy core meep-virt-engine -f` + recreate sandbox |
| Go code + Helm chart of MEC API service | `meepctl build meep-ams` | `meepctl dockerize meep-ams` + `meepctl dockerize meep-virt-engine` | `meepctl deploy core meep-virt-engine -f` + recreate sandbox |
| Go code of a core app (e.g. `go-apps/meep-platform-ctrl/`) | `meepctl build meep-platform-ctrl` | `meepctl dockerize meep-platform-ctrl` | `meepctl deploy core meep-platform-ctrl -f` |
| Helm chart of a core app (e.g. `charts/meep-platform-ctrl/`) | — | — | `meepctl deploy core meep-platform-ctrl -f` |
| Helm chart/values of a dependency (e.g. `charts/grafana/`) | — | — | `meepctl deploy dep meep-grafana -f` |

---

## Troubleshooting & Restart Procedures

### Restarting a Dependency (dep) App

Dependencies are third-party infrastructure services (databases, ingress, monitoring, etc.).

**Symptoms:** Grafana not loading, InfluxDB queries failing, Ingress 502 errors, Redis connection refused, etc.

```bash
# Check what's running
kubectl get pods | grep meep-

# Option 1: Force re-deploy (delete + redeploy) a specific dep
meepctl deploy dep meep-grafana --force

# Option 2: Delete and manually re-deploy
meepctl delete dep meep-grafana
# ... wait and investigate ...
meepctl deploy dep meep-grafana

# Option 3: Nuclear — restart ALL dependencies (disruptive!)
meepctl delete dep
meepctl deploy dep all
```

**Valid dep targets:**
`meep-cert-manager`, `meep-couchdb`, `meep-docker-registry`, `meep-grafana`, `meep-influxdb`, `meep-ingress`, `meep-kube-state-metrics`, `meep-open-map-tiles`, `meep-postgis`, `meep-prometheus`, `meep-redis`, `meep-thanos`, `meep-thanos-archive`

---

### Restarting a Core App

Core apps are the AdvantEDGE platform services.

**Symptoms:** Platform UI not accessible, monitoring dashboard empty, sandbox creation failing, webhook injection issues, etc.

```bash
# Force re-deploy a specific core app
meepctl deploy core meep-platform-ctrl --force

# Or delete + re-deploy manually
meepctl delete core meep-mon-engine
meepctl deploy core meep-mon-engine
```

**Valid core targets:**
`meep-auth-svc`, `meep-ingress-certs`, `meep-mon-engine`, `meep-platform-ctrl`, `meep-sandbox-api`, `meep-virt-engine`, `meep-webhook`

---

### Restarting a Sandbox MEC Service (API App)

Sandbox MEC services (meep-ams, meep-rnis, meep-loc-serv, etc.) are **not** managed by `meepctl deploy/delete` directly. They are deployed per-user by `meep-virt-engine` when a sandbox is created.

**To restart a MEC service:**

1. **From the MEC Sandbox UI:** Delete the user's sandbox and recreate it
2. **From the AdvantEDGE frontend:** Go to Exec page → terminate the scenario → re-deploy it
3. **Manually via Helm:**
   ```bash
   # List sandbox releases
   helm ls -A --short | grep <sandbox-name>
   
   # Delete the specific sandbox
   helm delete <sandbox-release-name>
   ```

**If you changed the service code:**
```bash
meepctl build meep-ams
meepctl dockerize meep-ams
# Then recreate the sandbox from the UI
```

**If you changed the service Helm chart:**
```bash
meepctl dockerize meep-virt-engine
meepctl deploy core meep-virt-engine --force
# Then recreate the sandbox from the UI
```

---

### Full Platform Restart

Use this procedure only when necessary (e.g., after a VM reboot or major upgrade):

```bash
# 1. Delete everything (reverse order)
meepctl delete core
meepctl delete dep

# 2. Rebuild if code changed
meepctl build all
meepctl dockerize all

# 3. Prune old images
docker image prune

# 4. Re-deploy (order matters: dep first, then core)
meepctl deploy dep all
# Wait for all dep pods to be Running:
kubectl get pods -w

meepctl deploy core all
```

---

## Common Failure Scenarios

### Pod stuck in CrashLoopBackOff

```bash
# Check logs
kubectl logs <pod-name> --previous

# Force re-deploy
meepctl deploy core <app-name> --force
# or
meepctl deploy dep <app-name> --force
```

### Pod stuck in Pending (resource issues)

```bash
# Check events
kubectl describe pod <pod-name>

# Common cause: PV/PVC issues — check storage
kubectl get pv,pvc
```

### Helm release stuck in "pending-install" or "failed" state

```bash
# Manually clean up
helm uninstall <release-name>

# Re-deploy
meepctl deploy dep <app-name>
# or
meepctl deploy core <app-name>
```

### meepctl reports "already deployed"

Use `--force` (or `-f`) to delete and re-deploy:

```bash
meepctl deploy core meep-platform-ctrl --force
meepctl deploy dep meep-grafana -f
```

### MEC API service not reflecting code changes

The most common cause is forgetting to dockerize after building:

```bash
# Make sure you did both:
meepctl build meep-ams
meepctl dockerize meep-ams

# Then recreate the sandbox from the UI
```

If you changed the Helm chart, remember to also update meep-virt-engine:

```bash
meepctl dockerize meep-virt-engine
meepctl deploy core meep-virt-engine --force
```

---

## MEC Service ↔ App Name Mapping

| MEC Standard | meepctl App Name | Description |
|-------------|-----------------|-------------|
| MEC011 | `meep-app-enablement` | MEC Application Support & Service Management |
| MEC012 | `meep-rnis` | Radio Network Information Service |
| MEC013 | `meep-loc-serv` | Location Service |
| MEC015 | `meep-tm` | Traffic Management (BWM + MTS) |
| MEC016 | `meep-dai` | Device Application Interface |
| MEC021 | `meep-ams` | Application Mobility Service |
| MEC028 | `meep-wais` | WLAN Access Information Service |
| MEC030 | `meep-vis` | V2X Information Service |
| MEC033 | `meep-iot` | IoT API |
| MEC040 | `meep-federation` | Federation Service |
| MEC046 | `meep-sss` | Sensors Sharing Service |
| — | `meep-sandbox-ctrl` | Sandbox Controller |
| — | `meep-gis-engine` | GIS Engine |
| — | `meep-metrics-engine` | Metrics Engine |
