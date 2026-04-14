## func cluster

Manage local development clusters

### Synopsis

Manage local Kubernetes development clusters with Knative, Tekton, and
other components pre-installed.

Create a fully configured development cluster:
  func cluster create

Create a minimal cluster (serving only):
  func cluster create --eventing=false

Create a full CI-style cluster:
  func cluster create --dapr --keda

Remove the cluster and associated resources:
  func cluster delete

### Options

```
  -h, --help   help for cluster
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions
* [func cluster create](func_cluster_create.md)	 - Create a local development cluster
* [func cluster delete](func_cluster_delete.md)	 - Delete a local development cluster
* [func cluster list](func_cluster_list.md)	 - List local development clusters

