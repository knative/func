# Local Kind Cluster - Knative Function Deployment

## Overview

This document covers the setup and troubleshooting of deploying Knative functions to a local kind cluster using remote builds with Tekton pipelines.

## Initial Setup

### Registry Configuration

**Issue**: Registry connection refused from inside cluster pods
- **Error**: `failed to ensure registry read access to localhost:50000/issue-744-go-func:latest: Get "https://localhost:50000/v2/": dial tcp [::1]:50000: connect: connection refused`
- **Root Cause**: Inside cluster pods, `localhost` refers to pod's loopback, not the host
- **Solution**: Use cluster-internal registry URL: `registry.default.svc.cluster.local:5000`

### Working Deployment Command

```bash
func deploy --remote --registry=registry.default.svc.cluster.local:5000
```

## DNS and Network Issues

### Issue 1: DNS Resolution for localtest.me

**Problem**: Function deployed but not accessible via browser at `http://issue-744-go-func.default.localtest.me`
- **Error**: `curl: (6) Could not resolve host: issue-744-go-func.default.localtest.me`
- **Root Cause**: Router DNS (192.168.178.1) was blocking/not resolving `localtest.me`
- **Contributing Factor**: Kernel update to 6.17.6/6.17.7 broke Docker port forwarding

### Issue 2: Localhost Port 80 Connection Reset

**Problem**: Connection reset when accessing via localhost
- **Error**: `curl: (56) Recv failure: Connection reset by peer`
- **Root Cause**: Kernel 6.17.7 has broken Docker localhost:80 port forwarding
- **Workaround**: Use LoadBalancer IP directly or port-forward

### Issue 3: MetalLB LoadBalancer Not Accessible from Host

**Problem**: LoadBalancer IP not accessible from host
- **Error**: `curl: (7) Failed to connect to 172.18.0.7 port 80 after 0 ms: Could not connect to server`
- **Root Cause**: unknown

### Access Workarounds

```bash
# Option 1: Use LoadBalancer IP directly (if not scaled to zero)
curl http://172.18.0.7 -H 'Host: issue-744-go-func.default.localtest.me'

# Option 2: Port-forward contour load balancer service
kubectl port-forward -n ??? service/??? 8080:80
curl http://localhost:8080 -H 'Host: issue-744-go-func.default.localtest.me'
```

## Cluster Provisioning

### allocate.sh Modifications

Modified `./hack/allocate.sh`:

1. **Registry ordering** (line 32): Added registry function before loadbalancer
2. **Registry cache directory** (lines 244-271): Added support for IMAGE_CACHE_DIR
3. **Registry readiness check** (lines 279-290): Added wait for registry to be ready
4. **Tekton affinity assistant fix** (lines 463-466):

```bash
echo "${blue}- Disabling affinity assistant (temporary workaround)${reset}"
$KUBECTL patch configmap feature-flags -n tekton-pipelines \
  -p '{"data":{"disable-affinity-assistant":"true", "coschedule":"disabled"}}' \
  --type=merge
```

## GitHub Actions Workflow

### Workflow File

Created `.github/workflows/remote-build-and-deploy.yaml` for automated deployment:

```yaml
name: Remote Build and Deploy

on:
  push:
    branches:
      - master
  workflow_dispatch:

jobs:
  deploy:
    runs-on: self-hosted
    steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Setup Kubernetes context
      uses: azure/k8s-set-context@v4
      with:
        method: kubeconfig
        kubeconfig: ${{ secrets.KUBECONFIG }}

    - name: Login to container registry
      if: ${{ vars.USE_REGISTRY_AUTH == 'true' }}
      uses: docker/login-action@v3
      with:
        registry: ${{ vars.REGISTRY_HOST }}
        username: ${{ secrets.REGISTRY_USERNAME }}
        password: ${{ secrets.REGISTRY_PASSWORD }}

    - name: Restore func cli from cache
      if: ${{ vars.USE_FUNC_CACHE == 'true' }}
      id: cache-func
      uses: actions/cache@v4
      with:
        path: func
        key: func-cli-knative-v1.19.1-${{ runner.os }}

    - name: Install func cli
      if: ${{ vars.USE_FUNC_CACHE != 'true' || steps.cache-func.outputs.cache-hit != 'true' }}
      uses: gauron99/knative-func-action@main
      with:
        version: knative-v1.19.1
        name: func

    - name: Deploy function
      run: func deploy --remote --registry=${{ vars.NAMESPACED_REGISTRY_URL }} -v ${{ vars.NAMESPACE }}
```

### GitHub Secrets and Variables

**Secrets**:
- `KUBECONFIG`: Base64-encoded kubeconfig file
- `REGISTRY_USERNAME`: Container registry username
- `REGISTRY_PASSWORD`: Container registry password

**Variables**:
- `REGISTRY_HOST`: Registry hostname (e.g., `docker.io` or empty for local)
- `NAMESPACED_REGISTRY_URL`: Full registry URL with namespace
- `NAMESPACE`: Kubernetes namespace for deployment
- `USE_REGISTRY_AUTH`: `true` or `false` to enable registry login
- `USE_FUNC_CACHE`: `true` or `false` to cache func CLI

### Setting Secrets via gh CLI

```bash
# Set kubeconfig
gh secret set KUBECONFIG < ~/.kube/config

# Set registry credentials
gh secret set REGISTRY_USERNAME -b 'username'
gh secret set REGISTRY_PASSWORD -b 'password'

# Set variables
gh variable set REGISTRY_HOST -b 'docker.io'
gh variable set NAMESPACED_REGISTRY_URL -b 'docker.io/mycompany'
gh variable set USE_REGISTRY_AUTH -b 'true'
```

## Key Learnings

1. **Registry URLs**: Inside cluster, use cluster-internal DNS names, not localhost
2. **Kernel Issues**: Fedora kernel 6.17.x has Docker networking regression
3. **MetalLB + Scale-to-Zero**: Interaction can prevent cold-start from external requests
4. **Tekton Affinity Assistant**: Must set both `disable-affinity-assistant: "true"` and `coschedule: "disabled"` for consistent behavior
5. **System Stability**: Avoid kernel changes, DNS changes, or other system-level modifications; use workarounds instead