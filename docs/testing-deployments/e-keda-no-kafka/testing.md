# Scenario E: KEDA, no Kafka

KEDA deployer, HTTP-only (same as today's default behavior).
Creates Deployment + Service + bridge Service + HTTPScaledObject.
No Kafka resources.

## Prerequisites

- kind, kubectl, Go 1.25+, Docker, jq

If using Colima and you hit DNS issues (image pulls failing, etc.), restart
it with explicit DNS:

```bash
colima stop
colima start --dns 8.8.8.8
```

## 1. Build the CLI

```bash
cd ~/go/src/knative.dev/func
go build -o /tmp/func-local ./cmd/func
```

## 2. Create cluster and install KEDA

```bash
kind create cluster --name test-e

kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.17.0/keda-2.17.0.yaml
kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.17.0/keda-2.17.0-core.yaml
kubectl wait deployment --all -n keda --for=condition=Available --timeout=120s

kubectl apply --server-side -f https://github.com/kedacore/http-add-on/releases/download/v0.12.0/keda-add-ons-http-0.12.0-crds.yaml
kubectl apply --server-side -f https://github.com/kedacore/http-add-on/releases/download/v0.12.0/keda-add-ons-http-0.12.0.yaml
kubectl wait deployment --all -n keda --for=condition=Available --timeout=120s
```

## 3. Create and configure the function

```bash
mkdir /tmp/test-keda-no-kafka && cd /tmp/test-keda-no-kafka
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-keda-no-kafka
runtime: go
registry: docker.io/aliok
deployer: keda
invoke: cloudevent
deploy:
  options:
    scale:
      min: 1
      max: 10
      keda:
        triggers:
          - type: http
```

## 4. Deploy

```bash
cd /tmp/test-keda-no-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 5. Verify

```bash
# Should exist: Deployment, Service, bridge Service, HTTPScaledObject
kubectl get deployment test-keda-no-kafka
kubectl get svc test-keda-no-kafka
kubectl get svc test-keda-no-kafka-interceptor-proxy
kubectl get httpscaledobject test-keda-no-kafka

# HTTPScaledObject details
kubectl get httpscaledobject test-keda-no-kafka -o json \
  | jq '{hosts: .spec.hosts, replicas: .spec.replicas, scaleTargetRef: .spec.scaleTargetRef}'

# Should NOT exist: no Kafka ScaledObject or TriggerAuthentication
kubectl get scaledobject test-keda-no-kafka-kafka 2>&1 || true
kubectl get triggerauthentication test-keda-no-kafka-kafka-auth 2>&1 || true

# No Kafka env vars
kubectl get deployment test-keda-no-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[]? | select(.name | startswith("KAFKA"))'
# Should print nothing
```

## 6. Collect resources

```bash
kubectl get deployment test-keda-no-kafka -o yaml > /tmp/test-keda-no-kafka/deployment.yaml
kubectl get svc test-keda-no-kafka -o yaml > /tmp/test-keda-no-kafka/service.yaml
kubectl get svc test-keda-no-kafka-interceptor-proxy -o yaml > /tmp/test-keda-no-kafka/bridge-service.yaml
kubectl get httpscaledobject test-keda-no-kafka -o yaml > /tmp/test-keda-no-kafka/httpscaledobject.yaml
```

## 7. Delete function

```bash
cd /tmp/test-keda-no-kafka
/tmp/func-local delete
```

## 8. Cleanup

```bash
kind delete cluster --name test-e
rm -rf /tmp/test-keda-no-kafka
```
