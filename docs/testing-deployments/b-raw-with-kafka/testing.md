# Scenario B: RAW, with Kafka

Raw deployer with Kafka (SASL_SSL). Deployment + Service with Kafka env vars
and CA cert volume. No KEDA resources.

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
kind create cluster --name test-b
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
mkdir /tmp/test-raw-kafka && cd /tmp/test-raw-kafka
/tmp/func-local create -l go -t cloudevents
```

Replace the contents of `func.yaml` (keep the `created` line from the generated file):

```yaml
created: <keep the generated value>
specVersion: 0.37.0
name: test-raw-kafka
runtime: go
registry: docker.io/aliok
deployer: raw
invoke: cloudevent
deploy:
  options:
    scale:
      min: 1
run:
  kafka:
    brokers: "my-cluster-kafka-bootstrap.kafka.svc.cluster.local:9093"
    topic: "test-topic"
    consumerGroup: "test-raw-kafka-group"
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
cd /tmp/test-raw-kafka
FUNC_REGISTRY=docker.io/aliok /tmp/func-local deploy --verbose
```

## 7. Verify

```bash
# Should exist: Deployment, Service
kubectl get deployment test-raw-kafka
kubectl get svc test-raw-kafka

# Should NOT exist: no KEDA resources (raw deployer)
kubectl get httpscaledobject test-raw-kafka 2>&1 || true
kubectl get scaledobject test-raw-kafka-kafka 2>&1 || true

# Kafka env vars should be present
kubectl get deployment test-raw-kafka -o json \
  | jq '.spec.template.spec.containers[0].env[] | select(.name | startswith("KAFKA") or . == "FUNC_TRANSPORT")'

# Volume mount for CA cert
kubectl get deployment test-raw-kafka -o json \
  | jq '.spec.template.spec.containers[0].volumeMounts'
```

## 8. Collect resources

```bash
kubectl get deployment test-raw-kafka -o yaml > /tmp/test-raw-kafka/deployment.yaml
kubectl get svc test-raw-kafka -o yaml > /tmp/test-raw-kafka/service.yaml
```

## 9. Delete function

```bash
cd /tmp/test-raw-kafka
/tmp/func-local delete
```

## 10. Cleanup

```bash
kind delete cluster --name test-b
rm -rf /tmp/test-raw-kafka
```
