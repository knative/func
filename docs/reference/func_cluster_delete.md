## func cluster delete

Delete a local development cluster

### Synopsis

Delete a local KinD development cluster and its associated registry container.

If only one cluster named "func" exists, it is deleted by default.
If multiple func-prefixed clusters exist, specify which one with --name.

EXAMPLES

  # Delete the default "func" cluster
  func cluster delete

  # Delete a named cluster
  func cluster delete --name myproject

```
func cluster delete [name]
```

### Options

```
      --container-engine string   Container engine: docker or podman ($FUNC_CONTAINER_ENGINE) (default "docker")
  -h, --help                      help for delete
  -n, --name string               Cluster name ($FUNC_CLUSTER_NAME) (default "func")
  -v, --verbose                   Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func cluster](func_cluster.md)	 - Manage local development clusters

