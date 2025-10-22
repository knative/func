# Remote OpenShift Cluster - Knative Function Deployment

## Overview

This document covers deploying Knative functions to a remote OpenShift cluster on AWS using the internal image registry and Tekton pipelines.

## Cluster Information

- **Cluster API**: `api.osdtests317.e4ab.p1.openshiftapps.com`
- **Namespace**: `issue-744-manually-created-ns`
- **Registry**: `image-registry.openshift-image-registry.svc:5000/issue-744-manually-created-ns`

## Prerequisites

### OpenShift Pipelines Operator

Tekton must be installed via the OpenShift Pipelines Operator:

```sh
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator
  namespace: openshift-operators
spec:
  channel: latest
  name: openshift-pipelines-operator-rh
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

oc get csv -n openshift-serverless -w

oc patch configmap feature-flags -n openshift-pipelines \
  -p '{"data":{"disable-affinity-assistant":"true", "coschedule":"disabled"}}' \
  --type=merge
```

## Authentication Issues and Solutions

### Issue 1: PVC Creation Unauthorized

**Error**:

```sh
Error: problem creating persistent volume claim: Unauthorized
```

**Root Cause**: Kubeconfig token expired (func uses user credentials from kubeconfig, not service account)

**Solution**: Login with `oc login` using username/password to get fresh token

```sh
# Login with username/password
oc login https://api.osdtests317.e4ab.p1.openshiftapps.com:6443 \
  --username=your-username \
  --password=your-password

# Token automatically added to ~/.kube/config
# Update GitHub secret
gh secret set KUBECONFIG < ~/.kube/config
```

**Code Location**: [pipelines_provider.go:562](../pkg/pipelines/tekton/pipelines_provider.go#L562) delegates to `k8s.CreatePersistentVolumeClaim`

### Issue 2: Registry Credentials Prompting

**Error**:

```sh
Please provide credentials for image repository 'default-route-openshift-image-registry.apps.osdtests317.e4ab.p1.openshiftapps.com/default/issue-744-go-func'.
Username:
```

**Root Cause**: Using external registry route instead of internal service name; automatic credential loading only works for `image-registry.openshift-image-registry.svc:5000`

**Solution**: Change registry URL to internal service with namespace suffix

```sh
# Correct registry URL
func deploy --remote --registry=image-registry.openshift-image-registry.svc:5000/issue-744-manually-created-ns
```

**Key Code Reference**: [openshift.go:88-94](../pkg/k8s/openshift.go#L88-L94)

```go
func GetDefaultOpenShiftRegistry() string {
	ns, _ := GetDefaultNamespace()
	if ns == "" {
		ns = "default"
	}
	return openShiftRegistryHostPort + "/" + ns
}
```

**Note**: OpenShift requires namespace suffix in registry URL; func appends it automatically when using internal registry

## Security Context and SCC Issues

### Issue: Volume Uploader Running as Root

**Error**:

```sh
container has runAsNonRoot and image will run as root (pod: "volume-uploader-jt7kp_default(...)", container: volume-uploader-jt7kp)
```

**Initial Diagnosis**: `Dockerfile.utils:18` has `USER 0:0`, OpenShift SCC blocks root containers

**Actual Root Cause**: Pods being created in wrong namespace with insufficient SCC permissions

**Solution**: Set current namespace in kubeconfig context

```sh
# Switch to correct namespace
oc project issue-744-manually-created-ns

# Update kubeconfig
# The oc client automatically updates ~/.kube/config current context

# Verify namespace is set
oc config get-contexts

# Update GitHub secret
gh secret set KUBECONFIG < ~/.kube/config
```

### Important Note on func-utils Image

The production `ghcr.io/knative/func-utils:v2` image is built from a downstream fork that's OpenShift-compatible:

- **Upstream**: https://github.com/knative/func (contains `Dockerfile.utils` with `USER 0:0`)
- **Downstream**: https://github.com/openshift-knative/kn-plugin-func (OpenShift-compatible builds)

**Key Code Reference**: [security_context.go:11-24](../pkg/k8s/security_context.go#L11-L24)

```go
func defaultPodSecurityContext() *corev1.PodSecurityContext {
	// change ownership of the mounted volume to the first non-root user uid=1000
	if IsOpenShift() {
		return nil  // Let OpenShift SCC assign UID
	}
	runAsUser := int64(1001)
	runAsGroup := int64(0)
	fsGroup := int64(1002)
	return &corev1.PodSecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
		FSGroup:    &fsGroup,
	}
}
```

## Tekton Affinity Assistant Issue

### Problem

**Error**:

```sh
0/10 nodes are available: 2 node(s) had untolerated taint {node-role.kubernetes.io/infra: }, 3 node(s) had untolerated taint {node-role.kubernetes.io/master: }, 5 node(s) didn't match pod affinity rules
```

**Root Cause**: Scaffold pod requires affinity with assistant pod `cfb2fd2bb6`, but only `cf7aff552a` exists; Tekton config has `disable-affinity-assistant: "true"` but `coschedule: workspaces` (mismatch)

### Configuration Mismatch

Check current Tekton configuration:

```sh
oc get configmap feature-flags -n openshift-pipelines -o yaml
```

**Problem Configuration**:

```yaml
data:
  disable-affinity-assistant: "true"
  coschedule: workspaces  # ← Should be "disabled"
```

**Correct Configuration** (matching working kind cluster):

```yaml
data:
  disable-affinity-assistant: "true"
  coschedule: disabled  # ← Must match disable-affinity-assistant
```

### Solution

The OpenShift Pipelines Operator allows ConfigMap modifications:

```bash
# Patch ConfigMap
oc patch configmap feature-flags -n openshift-pipelines \
  -p '{"data":{"disable-affinity-assistant":"true", "coschedule":"disabled"}}' \
  --type=merge


# Re-run deployment
func deploy --remote --registry=image-registry.openshift-image-registry.svc:5000/issue-744-manually-created-ns
```

**Related Issues**:

- https://github.com/tektoncd/pipeline/issues/6740
- https://github.com/tektoncd/pipeline/issues/7503

## GitHub Actions Integration

### Workflow Configuration for OpenShift

Use the same workflow file as local cluster, but configure different secrets/variables:

**Secrets**:

```sh
# Set OpenShift kubeconfig with current namespace set
gh secret set KUBECONFIG < ~/.kube/config
```

**Variables**:

```sh
# OpenShift internal registry with namespace
gh variable set NAMESPACED_REGISTRY_URL -b 'image-registry.openshift-image-registry.svc:5000/issue-744-manually-created-ns'

# Namespace for deployment
gh variable set NAMESPACE -b 'issue-744-manually-created-ns'

# Disable registry auth (uses kubeconfig token)
gh variable set USE_REGISTRY_AUTH -b 'false'

# No registry host needed for internal registry
gh variable delete REGISTRY_HOST
```

## Key Code References

### Registry Credential Loading

[openshift.go:97-125](../pkg/k8s/openshift.go#L97-L125) - `GetOpenShiftDockerCredentialLoaders`

- Extracts token from kubeconfig for registry auth
- Only matches internal registry hostname (`image-registry.openshift-image-registry.svc`)

### Credential Prompting

[prompt.go:18-83](../cmd/prompt/prompt.go#L18-L83) - `NewPromptForCredentials`

- Prompts user when credentials not found
- Called from [client.go:104-116](../cmd/client.go#L104-L116)

### Pipeline Execution Flow

[pipelines_provider.go](../pkg/pipelines/tekton/pipelines_provider.go):

1. Line 156: PVC creation happens first
2. Line 165: Volume upload to PVC
3. Line 186: Credentials requested AFTER volume upload
4. Line 203: Registry secret created with credentials

## Troubleshooting Checklist

- [ ] OpenShift Pipelines Operator installed
- [ ] Logged in with fresh token (`oc login`)
- [ ] Current namespace set in kubeconfig (`oc project <namespace>`)
- [ ] Using internal registry URL with namespace suffix
- [ ] Tekton feature flags properly configured (`coschedule: disabled`)
- [ ] User has permissions to create PVCs, pods, and pipeline resources
- [ ] GitHub secrets updated with fresh kubeconfig

## Key Learnings

1. **Namespace Context**: Always set current namespace in kubeconfig for correct SCC application
2. **Registry URL**: Must use internal service name with namespace suffix for automatic auth
3. **Token Expiration**: OpenShift tokens expire; use username/password login for fresh tokens
4. **Operator-Managed Resources**: ConfigMaps managed by operators can be modified; changes persist
5. **Tekton Configuration**: Both `disable-affinity-assistant` and `coschedule` must be aligned
6. **func-utils Image**: Production image is from downstream fork (openshift-knative), not upstream
