# Scenario A: RAW, no Kafka

Raw deployer, no Kafka. The simplest case: Deployment + Service, nothing else.

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

## 2. Create cluster

```bash
kind create cluster --name test-a
```

## 3. Create and configure the function

```bash
mkdir /tmp/test-raw-no-kafka && cd /tmp/test-raw-no-kafka
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-raw-no-kafka
runtime: go
registry: docker.io/aliok
deployer: raw
invoke: cloudevent
```

## 4. Deploy

```bash
cd /tmp/test-raw-no-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 5. Verify

```bash
# Should exist: Deployment, Service
kubectl get deployment test-raw-no-kafka
kubectl get svc test-raw-no-kafka

# Should NOT exist: HTTPScaledObject, ScaledObject, TriggerAuthentication, bridge Service
kubectl get httpscaledobject test-raw-no-kafka 2>&1 || true
kubectl get scaledobject test-raw-no-kafka-kafka 2>&1 || true
kubectl get triggerauthentication test-raw-no-kafka-kafka-auth 2>&1 || true
kubectl get svc test-raw-no-kafka-interceptor-proxy 2>&1 || true

# No Kafka env vars
kubectl get deployment test-raw-no-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[]? | select(.name | startswith("KAFKA"))'
# Should print nothing
```

## 6. Collect resources

```bash
kubectl get deployment test-raw-no-kafka -o yaml > /tmp/test-raw-no-kafka/deployment.yaml
kubectl get svc test-raw-no-kafka -o yaml > /tmp/test-raw-no-kafka/service.yaml
```

## 7. Delete function

```bash
cd /tmp/test-raw-no-kafka
/tmp/func-local delete
```

## 8. Cleanup

```bash
kind delete cluster --name test-a
rm -rf /tmp/test-raw-no-kafka
```
