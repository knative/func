package k8s

// DefaultClusterDomain is the DNS domain Kubernetes uses for in-cluster
// service names unless the cluster was configured with a different one.
const DefaultClusterDomain = "cluster.local"

// ClusterDomain returns the DNS domain used to build in-cluster service
// addresses such as <name>.<namespace>.svc.<domain>.
//
// Every deployer that builds such an address must go through this function
// instead of hard-coding "cluster.local". For now it always returns
// DefaultClusterDomain. Resolving the domain from a flag, the user config
// or the cluster itself will be added here once that contract is settled
// (see https://github.com/knative/func/issues/3403).
func ClusterDomain() string {
	return DefaultClusterDomain
}
