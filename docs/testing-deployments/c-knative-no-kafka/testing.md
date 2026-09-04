# Scenario C: Knative, no Kafka

Knative deployer with KPA scaling (using the new `kpa` sub-key).
Creates a Knative Service with autoscaling annotations.

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

## 2. Create cluster and install Knative Serving

```bash
kind create cluster --name test-c

kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-core.yaml
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.21.1/kourier.yaml
kubectl patch configmap/config-network -n knative-serving \
  --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
kubectl wait deployment --all -n knative-serving --for=condition=Available --timeout=120s
```

## 3. Create and configure the function

```bash
mkdir /tmp/test-knative-no-kafka && cd /tmp/test-knative-no-kafka
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-knative-no-kafka
runtime: go
registry: docker.io/aliok
deployer: knative
invoke: cloudevent
deploy:
  options:
    scale:
      min: 0
      max: 5
      kpa:
        metric: concurrency
        target: 100
        utilization: 70
```

## 4. Deploy

```bash
cd /tmp/test-knative-no-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 5. Verify

```bash
# Should exist: Knative Service (ksvc)
kubectl get ksvc test-knative-no-kafka

# Check KPA annotations
kubectl get ksvc test-knative-no-kafka -o json \
  | jq '.spec.template.metadata.annotations | {
      "autoscaling.knative.dev/minScale",
      "autoscaling.knative.dev/maxScale",
      "autoscaling.knative.dev/metric",
      "autoscaling.knative.dev/target",
      "autoscaling.knative.dev/target-utilization-percentage"
    }'

# Should NOT exist: no KEDA resources
kubectl get httpscaledobject test-knative-no-kafka 2>&1 || true
kubectl get scaledobject test-knative-no-kafka-kafka 2>&1 || true

# No Kafka env vars
kubectl get ksvc test-knative-no-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[]? | select(.name | startswith("KAFKA"))'
# Should print nothing
```

## 6. Collect resources

```bash
kubectl get ksvc test-knative-no-kafka -o yaml > /tmp/test-knative-no-kafka/ksvc.yaml
```

## 7. Delete function

```bash
cd /tmp/test-knative-no-kafka
/tmp/func-local delete
```

## 8. Cleanup

```bash
kind delete cluster --name test-c
rm -rf /tmp/test-knative-no-kafka
```
