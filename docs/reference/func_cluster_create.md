## func cluster create

Create a local development cluster

### Synopsis

Create a local KinD (Kubernetes in Docker) development cluster configured
for Knative Functions development. The cluster includes a local container
registry and can optionally include Knative Serving, Eventing, Tekton,
KEDA, and Dapr.

By default, a cluster with Knative Serving and Eventing is created —
suitable for most functions development. Enable Tekton with --tekton if
you also need in-cluster (remote) builds. Additional components
can be enabled with flags for CI/testing workflows.

EXAMPLES

  # Create a default development cluster
  func cluster create

  # Create a cluster with all components (for CI/E2E testing)
  func cluster create --dapr --keda

  # Create a minimal cluster (just Kubernetes + registry)
  func cluster create --serving=false --eventing=false

  # Create a cluster with a custom name and domain
  func cluster create --name myproject --domain example.local

  # Create with retries (useful in CI)
  FUNC_CLUSTER_RETRIES=3 func cluster create

```
func cluster create
```

### Options

```
      --container-engine string   Container engine: docker or podman ($FUNC_CONTAINER_ENGINE) (default "docker")
      --dapr                      Install Dapr runtime + Redis ($FUNC_CLUSTER_DAPR)
      --domain string             DNS domain for services ($FUNC_CLUSTER_DOMAIN) (default "localtest.me")
      --eventing                  Install Knative Eventing ($FUNC_CLUSTER_EVENTING) (default true)
  -h, --help                      help for create
      --keda                      Install KEDA + HTTP add-on ($FUNC_CLUSTER_KEDA)
  -n, --name string               Cluster name ($FUNC_CLUSTER_NAME) (default "func")
      --namespace string          Kubernetes namespace for RBAC bindings ($FUNC_NAMESPACE) (default "default")
      --no-cleanup                Don't delete cluster on failure ($FUNC_NO_CLEANUP)
      --pac-host string           PAC controller hostname ($FUNC_INT_PAC_HOST) (default "pac-ctr.localtest.me")
      --registry-port int         Local registry host port ($FUNC_REGISTRY_PORT) (default 50000)
      --retries int               Max cluster allocation attempts ($FUNC_CLUSTER_RETRIES) (default 1)
      --serving                   Install Knative Serving ($FUNC_CLUSTER_SERVING) (default true)
      --skip-binaries             Skip binary downloads ($FUNC_SKIP_BINARIES)
      --skip-registry-config      Skip host registry configuration ($FUNC_SKIP_REGISTRY_CONFIG)
      --tekton                    Install Tekton + Pipelines-as-Code ($FUNC_CLUSTER_TEKTON)
  -v, --verbose                   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func cluster](func_cluster.md)	 - Manage local development clusters

