# Provisioning a Kind (Kubernetes in Docker) Cluster

[kind](https://github.com/kubernetes-sigs/kind) is a lightweight tool for running local Kubernetes clusters using containers.  It can be used as the underlying infrastructure for Functions, though it is intended for testing and development rather than production deployment.

This guide walks through the process of configuring a kind cluster to run Functions with the following vserions:
* kind   v0.8.1 - [Install Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
* Kubectl v1.17.3 - [Install kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl) 

## Quickstart

Starting a new cluster with all defaults is as simple as:
```
kind create cluster
```
List available clusters:
```
kind get clusters
```
List running containers will now show a kind process:
```
docker ps
```
Confirm core services are running:
```
kubectl get po --all-namespaces
```
You should see


## Configure With Remote Access

This section is optional.

Kind is intended to be a locally-running service, and exposing externally is not recommended.  However, a fully configured kubernetes cluster can often quickly outstrip the resources available on even a well-specd development workstation.  Therefore, creating a Kind cluster network appliance of sorts can be helpful.  One possible way to connect to your kind cluster remotely would be to create a [wireguard](https://www.wireguard.com/) interface upon which to expose the API.  Following is an example assuming linux hosts with systemd:

### Create a Secure Tunnel

[Install Wireguard](https://www.wireguard.com/install/)

Create keypair for the host and client.
```
wg genkey | tee host.key | wg pubkey > host.pub
wg genkey | tee client.key | wg pubkey > client.pub
chmod 600 host.key client.key
```
Assuming IPv4 addresses, with the wireguard-protected network 10.10.10.0/24, the host being 10.10.10.1 and the client 10.10.10.2

Create a Wireguard Network Device on both the Host and the Client using the following configuration files (Replace HOST_KEY HOST_PUB, CLIENT_KEY and CLIENT_PUB with the keypairs created in the previous step):

On the Kind cluster host:
`/etc/systemd/network/99-wg0.netdev`
```
[NetDev]
Name=wg0
Kind=wireguard
Description=WireGuard tunnel wg0

[WireGuard]
ListenPort=51111
PrivateKey=HOST_KEY

[WireGuardPeer]
PublicKey=CLIENT_PUB
AllowedIPs=10.10.10.0/24
PersistentKeepalive=25
```

`/etc/systemd/network/99-wg0.network`
```
[Match]
Name=wg0

[Network]
Address=10.10.10.1/24
```

On the client:

`/etc/systemd/network/99-wg0.netdev`
```
[NetDev]
Name=wg0
Kind=wireguard
Description=WireGuard tunnel wg0

[WireGuard]
ListenPort=51871
PrivateKey=CLIENT_KEY

[WireGuardPeer]
PublicKey=HOST_PUB
AllowedIPs=10.10.10.0/24
Endpoint=HOST_ADDRESS:51111
PersistentKeepalive=25
```
Replace HOST_ADDRESS with an IP address at which the host can be reached prior to to wireguard interface becoming available.

`/etc/systemd/network/99-wg0.network`
```
[Match]
Name=wg0

[Network]
Address=10.10.10.2/24
```

_On both systems_, restrict the permissions of the network device file as it contains a sensitive private key, then restart systemd-networkd.
```
chown root:systemd-network /etc/systemd/network/99-*.netdev
chmod 0640 /etc/systemd/network/99-*.netdev
systemctl restart systemd-networkd
```
The nodes should now be able to ping each other using their wireguard-protected 10.10.10.0/24 addresses.  Additionally, statistics about the connection can be obtaned from the `wg` command:
```
wg show
```

### Provision the Cluster

Create a Kind configuration file which instructs the API server to listen on the Wireguard interface and a known port:
`kind-config.yaml`
```
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "10.10.10.1" # default is 127.0.0.1 (local only)
  apiServerPort: 6443 # default is random. Note this must be unique per cluster.
```

Delete the current cluster if necessary:
```
kind delete cluster --name kind
```
Start a new cluster using the config:
```
kind create cluster --config kind-config.yaml
```
Export a kubeconfig and move it to the client machine:
```
kind export kubeconfig --kubeconfig kind-kubeconfig.yaml
```
From the client, confirm that pods can be listed:
```
kubectl get po --all-namespaces --kubeconfig kind-kubeconfig.yaml
```
### Verify Cluster Provisioned

You should be able to retrieve nodes from the cluster, which should include coredns, kube-proxy, etc.
```
kubectl get po --all-namespaces
```

