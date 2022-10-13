# Getting Started with Kubernetes

Functions can be deployed to any kubernetes cluster which has been configured to support function workloads.  This guide details how to set up and configure a kubernetes cluster accordingly.

This guide was developed using the dependency versions listed in their requisite sections.  Instructions may deviate slightly as these projects are generally under active development.  It is recommended to use the links to the official documentation provided in each section.

## Provision a Cluster

Any Kubernetes-compatible API should be capable.  Included herein are instructions for two popular variants: Kind and EKS.

[Provision using Kind](provisioning/provision_kind.md)

[Provision using Amazon EKS](provisioning/provision_eks.md)

## Configuring the Cluster

Once access to a kubernetes-compatible cluster has been established, it will need to be configured to handle Function workloads.  This includes Knative Serving and Eventing, the Kourier networking layer, and CertManager with the LetsEncrypt certificate provider.

Create a namespace for your Functions:
```
kubectl create namespace func
```
Set the default namespace for subsequent commands:
```
kubectl config set-context --current --namespace=func
```

### Serving

Docs: https://knative.dev/docs/install/any-kubernetes-cluster/ )

Serving with Kourier networking
```
kubectl apply --filename https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-kourier/releases/download/knative-v1.5.0/kourier.yaml
```
Update the networking layer to
- Use Kourier
- use TLS
- Redirect HTTP requests to HTTPS
- Add func subdomain annotations
```
kubectl apply -f knative/config-network.yaml
```

Note: for environments where Load Balancers are not supported (such as local Kind clusters), the Kourier service will be stuck in a pending state as it is awaiting the underlying infrastructure to provision a load-balancer.  This can be solved by updating the Kourier configuration to type NodePort with its networking service attached to the host at ports HTTP 30080 and HTTPS 30443, you can use the following patch file:
```
kubectl patch -n kourier-system services/kourier -p "$(cat knative/config-kourier-nodeport.yaml)"
```
### Domains

Configure cluster-wide domain TLD+1 by editing k8s/config-domain.yaml to include supported domains.
First edit `knative/config-domain.yaml` to contain your domain of choice and then apply:
```
kubectl apply -f knative/config-domain.yaml
```
Note that this step is [pending automation](https://github.com/knative/func/issues/47)

### DNS

For external routing to the cluster, register domain(s) to be used with a registrar and configure a DNS CNAME to the DNS or IP returned from:
(May also register a wildcard subdomain match).
```
kubectl --namespace kourier-system get service kourier
```
For local installations such as Kind, the Kourier networking layer is configured as a local port, so DNS can be resolved by modifying one's local DNS resolver, or /etc/hosts, to point to the local host.

### TLS

In order to provision HTTPS routes, we optionally set up a Certificate Manager for the cluster.  In this example we configure it to use LetsEncrypt certificates vi a certificate provider and CloudFlare as the DNS provider.

#### Cert-Manager

Docs: https://cert-manager.io/docs/installation/kubernetes/
```
kubectl apply --validate=false -f https://github.com/cert-manager/cert-manager/releases/download/v1.9.0-beta.1/cert-manager.yaml
```
Create a Cluster Issuer by updating `tls/letsencrypt-issuer.yaml` with an email addresses for the LetsEncrypt registration and for the associated CloudFlare account:
```
kubectl apply -f tls/letsencrypt-issuer.yaml
```
Generate a token with CloudFlare with the following settings:
* Permission: Zone - Zone - Read
* Permission: Zone - DNS - Edit
* Zone Resources: Include - All ZOnes

Base64 encode the token:
```
echo -n "$CLOUDFLARE_TOKEN" | base64
```
Update the `tls/cloudflare-secret.yaml` with the base64-encoded token value and create the secret:
```
kubectl apply -f tls/cloudflare-secret.yaml
```

#### KNative Serving Cert-Manager Integration

Install the latest networking certmanager packaged with KNative Serving:
Docs: https://knative.dev/docs/serving/using-auto-tls/

```
kubectl apply --filename https://github.com/knative-sandbox/net-certmanager/releases/download/knative-v1.5.0/release.yaml
```
Edit config-certmanager to reference the letsencrypt issuer.  There should be an issuerRef pointing to a ClusterIssuer of name `letsencrypt-issuer`:
```
kubectl edit configmap config-certmanager --namespace knative-serving
```

### Eventing

Eventing with In-memory channels, a Channel broker, and enable the default broker in the func namespace.
```
kubectl apply --filename https://github.com/knative/eventing/releases/download/knative-v1.5.1/eventing-crds.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/knative-v1.5.1/eventing-core.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/knative-v1.5.1/in-memory-channel.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/knative-v1.5.1/mt-channel-broker.yaml
```
GitHub events source:
```
kubectl apply --filename https://github.com/knative/eventing-contrib/releases/download/v0.16.0/github.yaml
```
Learn more about the GitHub source at https://knative.dev/docs/eventing/samples/github-source/index.html

Enable Broker for func namespace:
```
kubectl label namespace func knative-eventing-injection=enabled
```

### Monitoring

Optionally the addition of the [metrics-server](https://github.com/kubernetes-sigs/metrics-server) API allows one to run `kubectl top nodes` and `kubectl top pods`.  It can be installed with:
```
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.3.7/components.yaml
```
Note that on local clusters such as Kind, it is necessary to add the following arguments to the metrics-server deployment in kube-system:
```
    args:
        - --kubelet-insecure-tls
        - --kubelet-preferred-address-types=InternalIP
```

### Troubleshooting

Get the installed KNative serving and Eventing versions
```
kubectl get namespace knative-serving -o 'go-template={{index .metadata.labels "serving.knative.dev/release"}}'
kubectl get namespace knative-eventing -o 'go-template={{index .metadata.labels "eventing.knative.dev/release"}}'
```


