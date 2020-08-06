# Provisioning an Amazon EKS (Elastic Kubernetes Service) Cluster

Amazon EKS is a fully managed Kubernetes service suitable for production deoployments.  The below instructions were compiled using the following dependency versions:

* eksctl       v1.15
* kubernetes   v1.15

[Offical EKS Documentation](https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html)

## AWS CLI tools

Install Python 3 via system package manager, and the AWS cli tools via pip:
```
pip install awscli --upgrade --user
```

## AWS Account

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

## SSH key

Generate cluster SSH key, saving into `./keys/ssh`
```
ssh-keygen -t rsa -b 4096
```

## Cluster Resources

Install `eksctl`
https://github.com/weaveworks/eksctl

Provision the cluster using `eksctl`.  For example, the configuration file `./eks/cluster-config.yaml` will create a single-node cluster named "prod" in the "us-west-2" region if used:
```
eksctl create cluster -f eks/config-cluster.yaml
```

## Users

Install users by modifying the template to include the ARN and username of the IAM users to give access to the cluster:
```
kubectl patch -n kube-system configmap/aws-auth --patch "$(cat eks/users.yaml)"
```

## Verify Cluster Provisioned

You should be able to retrieve nodes from the cluster, which should include coredns, kube-proxy, etc.
```
kubectl get po --all-namespaces
```

## Administration

See the [eksctl](https://eksctl.io) documentation for how to adminster a cluster, such as [cluster upgrades](https://eksctl.io/usage/cluster-upgrade/) using this helper CLI.

