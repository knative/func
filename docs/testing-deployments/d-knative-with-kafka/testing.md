# Scenario D: Knative, with Kafka

Knative deployer with Kafka (SASL_SSL) and KPA scaling.
Creates a Knative Service with Kafka env vars/volumes and autoscaling
annotations. No KEDA resources.

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
kind create cluster --name test-d

kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.21.2/serving-core.yaml
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.21.1/kourier.yaml
kubectl patch configmap/config-network -n knative-serving \
  --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
kubectl wait deployment --all -n knative-serving --for=condition=Available --timeout=120s
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
mkdir /tmp/test-knative-kafka && cd /tmp/test-knative-kafka
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-knative-kafka
runtime: go
registry: docker.io/aliok
deployer: knative
invoke: cloudevent
deploy:
  options:
    scale:
      min: 1
      max: 10
      kpa:
        metric: concurrency
        target: 50
run:
  kafka:
    brokers: "my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9093"
    topic: "test-topic"
    consumerGroup: "test-knative-kafka-group"
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
cd /tmp/test-knative-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 7. Verify

```bash
# Should exist: Knative Service
kubectl get ksvc test-knative-kafka

# KPA annotations
kubectl get ksvc test-knative-kafka -o json \
  | jq '.spec.template.metadata.annotations | {
      "autoscaling.knative.dev/minScale",
      "autoscaling.knative.dev/maxScale",
      "autoscaling.knative.dev/metric",
      "autoscaling.knative.dev/target"
    }'

# Kafka env vars should be present
kubectl get ksvc test-knative-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[] | select(.name | startswith("KAFKA") or . == "FUNC_TRANSPORT")'

# Volume mount for CA cert
kubectl get ksvc test-knative-kafka -o json \
  | jq '.spec.template.spec.containers[0].volumeMounts'

# Should NOT exist: no KEDA resources (Knative deployer)
kubectl get scaledobject test-knative-kafka-kafka 2>&1 || true
```

## 8. Collect resources

```bash
kubectl get ksvc test-knative-kafka -o yaml > /tmp/test-knative-kafka/ksvc.yaml
```

## 9. Delete function

```bash
cd /tmp/test-knative-kafka
/tmp/func-local delete
```

## 10. Cleanup

```bash
kind delete cluster --name test-d
rm -rf /tmp/test-knative-kafka
```
