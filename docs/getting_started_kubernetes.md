# Getting Started with Kubernetes

Functions can be deployed to any kubernetes cluster which has been configured to support function workloads.  This guide details how to set up and configure a kubernetes cluster accordingly.

This guide was developed using the dependency versions listed in their requisite sections.  Instructions may deviate slightly as these projects are generally under active development.  It is recommended to use the links to the official documentation provided in each section.

Any Kubernetes-compatible API should be capable.  Included herein are instructions for two popular variants: Kind and EKS.

## Local dependencies

This guide assumes the following local shared dependncies:
* Kubectl v1.17.3 - [Install `kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl) 

## Provisioning a Kind (Kubernetes in Docker) Cluster

[kind](https://github.com/kubernetes-sigs/kind) is a lightweight tool for running local Kubernetes clusters using containers.  It can be used as the underlying infrastructure for Functions, though it is intended for testing and development rather than production deployment.

This guide walks through the process of configuring a kind cluster to run Functions with the following vserions:
* kind   v0.8.1 - [Install Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)

Start a new cluster:
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

### Connecting Remotely

Kind is intended to be a locally-running service, and exposing externally is not recommended.  However, a fully configured kubernetes cluster can often quickly outstrip the resources available on even a well-specced development workstation.  Therefore, creating a Kind cluster network appliance of sorts can be helpful.  One possible way to connect to your kind cluster remotely would be to create a [wireguard](https://www.wireguard.com/) interface upon which to expose the API.  Following is an example assuming linux hosts with systemd:

First [Install Wireguard](https://www.wireguard.com/install/)

Create keypair for the host and client.
```
wg genkey | tee host.key | wg pubkey > host.pub
wg genkey | tee client.key | wg pubkey > client.pub
chmod 600 host.key client.key
```
Assuming IPv4 addresses, with the wireguard-protected network 10.10.10.0/24, the host being 10.10.10.1 and the client 10.10.10.2

On the host, create a Wireguard Network Device:
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
PublicKey=HOST_PUB
AllowedIPs=10.10.10.0/24
PersistentKeepalive=25
```
(Replace HOST_KEY and HOST_PUB with the keypair created earlier.)

`/etc/systemd/network/99-wg0.network`
```
[Match]
Name=wg0

[Network]
Address=10.10.10.1/24
```

On the client, create the Wireguard Network Device and Network:
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
PublicKey=CLIENT_PUB
AllowedIPs=10.10.10.0/24
Endpoint=HOST_ADDRESS:51111
PersistentKeepalive=25
```
(Replace HOST_KEY and HOST_PUB with the keypair created earlier.)

Replace HOST_ADDRESS with an IP address at which the host can be reached prior to to wireguard interface becoming available.

`/etc/systemd/network/99-wg0.network`
```
[Match]
Name=wg0

[Network]
Address=10.10.10.2/24
```

_On both systems_, restrict the permissions of the network device file as it contains sensitive keys, then restart systemd-networkd.
```
chown root:systemd-network /etc/systemd/network/99-*.netdev
chmod 0640 /etc/systemd/network/99-*.netdev
systemctl restart systemd-networkd
```

The hosts should now be able to ping each other using their wireguard-protectd 10.10.10.0/24 addresses.  Additionally, statistics about the connection can be obtaned from the `wg` command:
```
wg show
```

Create a Kind configuration file which instructs the API server to listen on the Wireguard interface and a known port:
`kind-config.yaml`
```
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "10.10.10.1" # default 127.0.0.1
  apiServerPort: 6443 # default random, must be different for each cluster
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

## Provisioning an EKS (Elastic Kubernetes Service) Cluster

Amazon EKS is a fully managed Kubernetes service suitable for production deoployments.  The below instructions were compiled using the following dependency versions:

* eksctl       v1.15
* kubernetes   v1.15

[Offical EKS Documentation](https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html)

### AWS CLI tools

Install Python 3 via system package manager, and the AWS cli tools via pip:
```
pip install awscli --upgrade --user
```

### AWS Account

Install the AWS IAM Authenticator
https://github.com/kubernetes-sigs/aws-iam-authenticator

Create AWS account(s) via the AWS console:

https://console.aws.amazon.com/

Users _of_ the cluster require no permissions at this point, but the user _creating_ the cluster does.  Once configured, set the local environment variables:
```
AWS_REGION=us-east-2
AWS_SECRET_ACCESS_KEY=[redacted]
AWS_ACCESS_KEY_ID=[redacted]
```

Or use aws credentials.  To configure the CLI to use credintials, for instance:

To `~/.aws/config` append:

```
[profile alice]
region = us-west-2
output = json
```

To `~/.aws/credentials` append:

```
[alice]
aws_access_key_id = [redacted]
aws_secret_access_key = [redacted]
```

The profile to use can then be configured using the environment varaible:

AWS_PROFILE=alice

(note that [direnv](https://direnv.net/) can be handy here.)

### SSH key

Generate cluster SSH key, saving into `./keys/ssh`
```
ssh-keygen -t rsa -b 4096
```

### Cluster Resources

Install `eksctl`
https://github.com/weaveworks/eksctl

Provision the cluster using `eksctl`.  For example, the configuration file `./eks/cluster-config.yaml` will create a single-node cluster named "prod" in the "us-west-2" region if used:
```
eksctl create cluster -f eks/config-cluster.yaml
```

### Verify Cluster Provisioned

You should be able to retrieve nodes from the cluster
```
kubectl get po --all-namespaces
```

### Administration

See the [eksctl](https://eksctl.io) documentation for how to adminster a cluster, such as [cluster upgrades](https://eksctl.io/usage/cluster-upgrade/) using this helper CLI.

## Configuring the Cluster

Once access to a kubernetes-compatible cluster has been established, it will need to be configured to handle Function workloads.  This includes Knative Serving and Eventing, the Kourier networking layer, and CertManager with the LetsEncrypt certificate provider.

### Serving

Docs: https://knative.dev/docs/install/any-kubernetes-cluster/ ) 

Serving with Kourier networking
```
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.16.0/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.16.0/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-kourier/releases/download/v0.16.0/kourier.yaml
```
Update the networking layer to
- Use Kourier
- use TLS
- Redirect HTTP requests to HTTPS
- Add faas subdomain annotations
```
kubectl apply -f knative/config-network.yaml
```

Note: for environments where Load Balancers are not supported (such as local Kind clusters), the Kourier service should be updated to be of type IP instead of LoadBalancer.

### Domains

Configure cluster-wide domain TLD+1 by editing k8s/config-domain.yaml to include supported domains.
Update the `knative/config-domain.yaml` to contain your domain of choice and then apply:
```
kubectl apply -f knative/config-domain.yaml
```
Note that this step is [pending automation](https://github.com/boson-project/faas/issues/47)

### DNS 

Register domain(s) to be used, configuring a CNAME to the DNS or IP returned from:
```
kubectl --namespace kourier-system get service kourier
```
May register a wildcard matching subdomain, for example.

### Users

Install users:
```
kubectl patch -n kube-system configmap/aws-auth --patch "$(cat users.yaml)"
```

### TLS

Assumed Cert Manager configured to use Letsencrypt production and CloudFlare for DNS.

Install Cert-manager

Docs: https://cert-manager.io/docs/installation/kubernetes/
```
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.14.3/cert-manager.yaml
```

Create a Cluster Issuer, update cluster-issuer.yaml with email address, and create the associated cloudflare secret.
```
kubectl apply -f cluster-issuer.yaml
```
The secret should be a CloudFlare Token with Zone Read and DNS Write permissions, in the UI as:
```
Zone -> Zone -> Read
Zone -> DNS -> Edit
```
```
kubectl apply -f secrets/cloudflare.yaml
```
Install the latest networking certmanager:
```
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.13.0/serving-cert-manager.yaml
```
Edit config-certmanager to reference the letsencrypt issuer:
```
kubectl edit configmap config-certmanager --namespace knative-serving
```


### Eventing

Eventing with In-memory channels, a Channel broker, and enable the default broker in the faas namespace.
```
kubectl apply --filename https://github.com/knative/eventing/releases/download/v0.13.0/eventing-crds.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/v0.13.0/eventing-core.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/v0.13.0/in-memory-channel.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/v0.13.0/channel-broker.yaml
```
Enable Broker for faas namespace and install GitHub source:
```
kubectl create namespace faas
kubectl label namespace faas knative-eventing-injection=enabled
kubectl apply --filename https://github.com/knative/eventing-contrib/releases/download/v0.13.0/github.yaml
```
Learn more about the Github source at https://knative.dev/docs/eventing/samples/github-source/index.html


### Other

Get serving version
```
kubectl get namespace knative-serving -o 'go-template={{index .metadata.labels "serving.knative.dev/release"}}'
```

Get eventing version
```
kubectl get namespace knative-eventing -o 'go-template={{index .metadata.labels "eventing.knative.dev/release"}}'
```



