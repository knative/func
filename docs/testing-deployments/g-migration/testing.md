# Scenario G: Migration (old flat scale fields to kpa sub-key)

Tests that an old-format func.yaml (specVersion 0.36.0 with flat
metric/target/utilization) gets migrated to the `kpa` sub-key on deploy.

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
kind create cluster --name test-g

kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-core.yaml
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.21.1/kourier.yaml
kubectl patch configmap/config-network -n knative-serving \
  --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
kubectl wait deployment --all -n knative-serving --for=condition=Available --timeout=120s
```

## 3. Create and configure the function

```bash
mkdir /tmp/test-migration && cd /tmp/test-migration
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file) — use OLD specVersion and flat fields:

```yaml
created: <keep the generated value>
specVersion: 0.36.0
name: test-migration
runtime: go
registry: docker.io/aliok
deployer: knative
invoke: cloudevent
deploy:
  options:
    scale:
      min: 1
      max: 5
      metric: concurrency
      target: 200
      utilization: 80
```

## 4. Deploy

```bash
cd /tmp/test-migration
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 5. Verify

Check that func.yaml was migrated:

```bash
cat /tmp/test-migration/func.yaml
# Should show:
#   specVersion: 0.37.0
#   kpa:
#     metric: concurrency
#     target: 200
#     utilization: 80
#   (flat metric/target/utilization also still present for backwards compat)
```

Check that KPA annotations are correct on the Knative Service:

```bash
kubectl get ksvc test-migration -o json \
  | jq '.spec.template.metadata.annotations | {
      "autoscaling.knative.dev/metric",
      "autoscaling.knative.dev/target",
      "autoscaling.knative.dev/target-utilization-percentage"
    }'
```

## 6. Delete function

```bash
cd /tmp/test-migration
/tmp/func-local delete
```

## 7. Cleanup

```bash
kind delete cluster --name test-g
rm -rf /tmp/test-migration
```
