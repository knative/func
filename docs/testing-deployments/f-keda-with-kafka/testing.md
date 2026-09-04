# Scenario F: KEDA, with Kafka (Kafka-only trigger, scaling test)

KEDA deployer with Kafka (SASL_SSL) and explicit Kafka trigger.
Tests that the function scales up when messages are produced to the topic.

Creates: Deployment, Service, **ScaledObject**, **TriggerAuthentication**.

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
kind create cluster --name test-f

kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.17.0/keda-2.17.0.yaml
kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.17.0/keda-2.17.0-core.yaml
kubectl wait deployment --all -n keda --for=condition=Available --timeout=120s

kubectl apply --server-side -f https://github.com/kedacore/http-add-on/releases/download/v0.12.0/keda-add-ons-http-0.12.0-crds.yaml
kubectl apply --server-side -f https://github.com/kedacore/http-add-on/releases/download/v0.12.0/keda-add-ons-http-0.12.0.yaml
kubectl wait deployment --all -n keda --for=condition=Available --timeout=120s
```

## 3. Install Strimzi and create Kafka cluster + user + topic

```bash
kubectl create namespace kafka
kubectl apply -f 'https://strimzi.io/install/latest?namespace=kafka' -n kafka
kubectl wait --for=condition=Ready pods --all -n kafka --timeout=120s

kubectl apply -n kafka -f - <<'EOF'
apiVersion: kafka.strimzi.io/v1
kind: KafkaNodePool
metadata:
  name: dual-role
  labels:
    strimzi.io/cluster: my-cluster
spec:
  replicas: 1
  roles: [controller, broker]
  storage:
    type: jbod
    volumes:
      - id: 0
        type: persistent-claim
        size: 1Gi
        deleteClaim: true
---
apiVersion: kafka.strimzi.io/v1
kind: Kafka
metadata:
  name: my-cluster
  annotations:
    strimzi.io/node-pools: enabled
    strimzi.io/kraft: enabled
spec:
  kafka:
    version: 4.2.0
    authorization:
      type: simple
      superUsers: [ANONYMOUS]
    listeners:
      - name: plain
        port: 9092
        type: internal
        tls: false
      - name: tls
        port: 9093
        type: internal
        tls: true
        authentication:
          type: scram-sha-512
    config:
      offsets.topic.replication.factor: 1
      transaction.state.log.replication.factor: 1
      transaction.state.log.min.isr: 1
  entityOperator:
    topicOperator: {}
    userOperator: {}
EOF

kubectl wait kafka/my-cluster --for=condition=Ready --timeout=300s -n kafka

kubectl apply -n kafka -f - <<'EOF'
apiVersion: kafka.strimzi.io/v1
kind: KafkaUser
metadata:
  name: my-kafka-user
  labels:
    strimzi.io/cluster: my-cluster
spec:
  authentication:
    type: scram-sha-512
  authorization:
    type: simple
    acls:
      - resource: { type: topic, name: test-topic, patternType: literal }
        operations: [Read, Describe]
        host: "*"
      - resource: { type: group, name: "*", patternType: literal }
        operations: [Read]
        host: "*"
---
apiVersion: kafka.strimzi.io/v1
kind: KafkaTopic
metadata:
  name: test-topic
  labels:
    strimzi.io/cluster: my-cluster
spec:
  partitions: 3
  replicas: 1
EOF

kubectl wait kafkauser/my-kafka-user --for=condition=Ready --timeout=60s -n kafka
```

## 4. Copy secrets to default namespace

```bash
kubectl get secret my-cluster-cluster-ca-cert -n kafka -o json \
  | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid,.metadata.creationTimestamp,.metadata.ownerReferences)' \
  | kubectl apply -n default -f -

kubectl get secret my-kafka-user -n kafka -o json \
  | jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid,.metadata.creationTimestamp,.metadata.ownerReferences)' \
  | kubectl apply -n default -f -
```

## 5. Create and configure the function

```bash
mkdir /tmp/test-keda-kafka && cd /tmp/test-keda-kafka
/tmp/func-local create -l go -t cloudevents
```

Add a sleep to `function.go` so messages take time to process (otherwise the
consumer is too fast and KEDA never sees lag):

```go
// In the Handle method, add after the fmt.Println lines:
time.Sleep(5 * time.Second)
```

And add `"time"` to the imports.

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-keda-kafka
runtime: go
registry: docker.io/aliok
deployer: keda
invoke: cloudevent
deploy:
  options:
    scale:
      min: 0
      max: 10
      keda:
        triggers:
          - type: kafka
            lagThreshold: 5
            activationLagThreshold: 0
run:
  kafka:
    brokers: "my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9093"
    topic: "test-topic"
    consumerGroup: "test-keda-kafka-group"
    securityProtocol: "SASL_SSL"
    tls:
      caCert: "/etc/kafka/ca/ca.crt"
    sasl:
      mechanism: "SCRAM-SHA-512"
      user: "my-kafka-user"
      password: "{{ secret:my-kafka-user:password }}"
  volumes:
    - secret: my-cluster-cluster-ca-cert
      path: /etc/kafka/ca
```

## 6. Deploy

```bash
cd /tmp/test-keda-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 7. Verify

```bash
# Should exist: Deployment, Service, ScaledObject, TriggerAuthentication
kubectl get deployment test-keda-kafka
kubectl get svc test-keda-kafka
kubectl get scaledobject test-keda-kafka-kafka
kubectl get triggerauthentication test-keda-kafka-kafka-auth

# Should NOT exist: no HTTP resources (Kafka-only trigger)
kubectl get svc test-keda-kafka-interceptor-bridge 2>&1 || true
kubectl get httpscaledobject test-keda-kafka 2>&1 || true

# ScaledObject details
kubectl get scaledobject test-keda-kafka-kafka -o json | jq '{
  scaleTargetRef: .spec.scaleTargetRef,
  minReplicaCount: .spec.minReplicaCount,
  maxReplicaCount: .spec.maxReplicaCount,
  triggers: .spec.triggers
}'

# TriggerAuthentication details
kubectl get triggerauthentication test-keda-kafka-kafka-auth -o json | jq '{
  secretTargetRef: .spec.secretTargetRef
}'

# Kafka env vars should be present on the Deployment
kubectl get deployment test-keda-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[] | select(.name | startswith("KAFKA") or . == "FUNC_TRANSPORT")'

# Owner references should point to the Deployment
kubectl get scaledobject test-keda-kafka-kafka -o json | jq '.metadata.ownerReferences'
kubectl get triggerauthentication test-keda-kafka-kafka-auth -o json | jq '.metadata.ownerReferences'
```

## 8. Test Kafka scaling

With `min: 0` and `lagThreshold: 5`, KEDA will scale from 0 to up to 10
replicas based on consumer lag. The topic has 3 partitions, so max effective
replicas from Kafka alone is 3 (one consumer per partition).

```bash
# Check current replicas — should be 0 (scaled to zero, no lag)
kubectl get deployment test-keda-kafka -o jsonpath='{.spec.replicas}'
echo

# In a separate terminal, watch replicas:
# kubectl get deployment test-keda-kafka -w

# Flood the topic with 10000 messages (uses the plain listener, no auth needed)
kubectl run kafka-producer -n kafka --rm -i --restart=Never \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-console-producer.sh \
    --bootstrap-server my-cluster-kafka-bootstrap:9092 \
    --topic test-topic \
    <<< "$(for i in $(seq 1 10000); do echo "message-$i"; done)"

# Wait 30-60 seconds, then check replicas — should have scaled up
kubectl get deployment test-keda-kafka

# Check consumer lag (the function's consumer group)
kubectl run kafka-lag -n kafka --rm -i --restart=Never \
  --image=quay.io/strimzi/kafka:latest-kafka-4.2.0 -- \
  bin/kafka-consumer-groups.sh \
    --bootstrap-server my-cluster-kafka-bootstrap:9092 \
    --describe --group test-keda-kafka-group
```

## 9. Collect resources

```bash
kubectl get deployment test-keda-kafka -o yaml > /tmp/test-keda-kafka/deployment.yaml
kubectl get svc test-keda-kafka -o yaml > /tmp/test-keda-kafka/service.yaml
kubectl get scaledobject test-keda-kafka-kafka -o yaml > /tmp/test-keda-kafka/scaledobject.yaml
kubectl get triggerauthentication test-keda-kafka-kafka-auth -o yaml > /tmp/test-keda-kafka/triggerauthentication.yaml
```

## 10. Delete function

```bash
cd /tmp/test-keda-kafka
/tmp/func-local delete
```

Verify cleanup:

```bash
kubectl get scaledobject test-keda-kafka-kafka 2>&1 || true
kubectl get triggerauthentication test-keda-kafka-kafka-auth 2>&1 || true
# Both should return "not found"
```

## 11. Cleanup

```bash
kind delete cluster --name test-f
rm -rf /tmp/test-keda-kafka
```
