package keda

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	fn "knative.dev/func/pkg/functions"
)

var (
	scaledObjectGVR = schema.GroupVersionResource{
		Group:    "keda.sh",
		Version:  "v1alpha1",
		Resource: "scaledobjects",
	}
	triggerAuthGVR = schema.GroupVersionResource{
		Group:    "keda.sh",
		Version:  "v1alpha1",
		Resource: "triggerauthentications",
	}
)

func scaledObjectName(funcName string) string {
	return funcName + "-kafka"
}

func triggerAuthName(funcName string) string {
	return funcName + "-kafka-auth"
}

func triggers(f fn.Function) []fn.KEDATrigger {
	if f.Deploy.Options.Scale != nil && f.Deploy.Options.Scale.KEDA != nil {
		return f.Deploy.Options.Scale.KEDA.Triggers
	}
	return nil
}

func hasHTTPTrigger(triggers []fn.KEDATrigger) bool {
	for _, t := range triggers {
		if t.Type == "http" {
			return true
		}
	}
	return false
}

func hasKafkaTrigger(triggers []fn.KEDATrigger) bool {
	for _, t := range triggers {
		if t.Type == "kafka" {
			return true
		}
	}
	return false
}

func kafkaTrigger(triggers []fn.KEDATrigger) fn.KEDATrigger {
	for _, t := range triggers {
		if t.Type == "kafka" {
			return t
		}
	}
	return fn.KEDATrigger{}
}

// needsTriggerAuth returns true when the Kafka config uses SASL or TLS with
// secrets that must be referenced via a TriggerAuthentication.
func needsTriggerAuth(kafka *fn.KafkaConfig) bool {
	if kafka == nil {
		return false
	}
	if kafka.SASL != nil && kafka.SASL.Password != "" {
		return true
	}
	if kafka.TLS != nil && kafka.TLS.CACert != "" {
		return true
	}
	return false
}

// parseSecretRef extracts the secret name and key from a {{ secret:name:key }}
// reference. Returns empty strings if the value is not a secret reference.
func parseSecretRef(value string) (secretName, secretKey string) {
	if !strings.HasPrefix(value, "{{") {
		return
	}
	trimmed := strings.Trim(value, "{} ")
	parts := strings.Split(trimmed, ":")
	if len(parts) == 3 && strings.TrimSpace(parts[0]) == "secret" {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	}
	return
}

// findSecretForPath finds the volume secret name that backs a given file path.
// It matches by checking which volume's mount path is a parent of the cert path.
func findSecretForPath(certPath string, volumes []fn.Volume) (secretName, key string) {
	for _, v := range volumes {
		if v.Secret == nil || v.Path == nil {
			continue
		}
		mountPath := *v.Path
		if strings.HasPrefix(certPath, mountPath) {
			rel, err := filepath.Rel(mountPath, certPath)
			if err != nil {
				continue
			}
			return *v.Secret, rel
		}
	}
	return
}

// buildTriggerAuth creates the unstructured TriggerAuthentication for Kafka SASL/TLS.
func buildTriggerAuth(f fn.Function, deployment *v1.Deployment, namespace string) *unstructured.Unstructured {
	kafka := f.Run.Kafka
	if kafka == nil {
		return nil
	}

	var secretRefs []interface{}
	var envRefs []interface{}

	if kafka.SASL != nil && kafka.SASL.Password != "" {
		secretName, secretKey := parseSecretRef(kafka.SASL.Password)
		if secretName != "" {
			secretRefs = append(secretRefs, map[string]interface{}{
				"parameter": "password",
				"name":      secretName,
				"key":       secretKey,
			})
		}

		if kafka.SASL.User != "" {
			userName, userKey := parseSecretRef(kafka.SASL.User)
			if userName != "" {
				secretRefs = append(secretRefs, map[string]interface{}{
					"parameter": "username",
					"name":      userName,
					"key":       userKey,
				})
			} else {
				envRefs = append(envRefs, map[string]interface{}{
					"parameter":     "username",
					"name":          "KAFKA_SASL_USER",
					"containerName": deployment.Spec.Template.Spec.Containers[0].Name,
				})
			}
		}
	}

	if kafka.TLS != nil && kafka.TLS.CACert != "" {
		caSecretName, caKey := findSecretForPath(kafka.TLS.CACert, f.Run.Volumes)
		if caSecretName != "" {
			secretRefs = append(secretRefs, map[string]interface{}{
				"parameter": "ca",
				"name":      caSecretName,
				"key":       caKey,
			})
		}
	}

	if len(secretRefs) == 0 && len(envRefs) == 0 {
		return nil
	}

	spec := map[string]interface{}{}
	if len(secretRefs) > 0 {
		spec["secretTargetRef"] = secretRefs
	}
	if len(envRefs) > 0 {
		spec["env"] = envRefs
	}

	ta := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "TriggerAuthentication",
			"metadata": map[string]interface{}{
				"name":      triggerAuthName(f.Name),
				"namespace": namespace,
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "apps/v1",
						"kind":               "Deployment",
						"name":               deployment.Name,
						"uid":                string(deployment.UID),
						"controller":         true,
						"blockOwnerDeletion": true,
					},
				},
			},
			"spec": spec,
		},
	}

	return ta
}

// kedaSASLType maps func.yaml SASL mechanism names to KEDA trigger metadata values.
func kedaSASLType(mechanism string) string {
	switch mechanism {
	case "SCRAM-SHA-256":
		return "scram_sha256"
	case "SCRAM-SHA-512":
		return "scram_sha512"
	case "PLAIN":
		return "plain"
	default:
		return ""
	}
}

// buildScaledObject creates the unstructured ScaledObject for Kafka consumer-lag scaling.
func buildScaledObject(f fn.Function, trigger fn.KEDATrigger, deployment *v1.Deployment, namespace string, minScale, maxScale int32) *unstructured.Unstructured {
	kafka := f.Run.Kafka
	if kafka == nil {
		return nil
	}

	lagThreshold := int64(10)
	if trigger.LagThreshold != nil {
		lagThreshold = *trigger.LagThreshold
	}

	triggerMeta := map[string]interface{}{
		"bootstrapServers": kafka.Brokers,
		"consumerGroup":    kafka.ConsumerGroup,
		"topic":            kafka.Topic,
		"lagThreshold":     fmt.Sprintf("%d", lagThreshold),
	}

	if trigger.ActivationLagThreshold != nil {
		triggerMeta["activationLagThreshold"] = fmt.Sprintf("%d", *trigger.ActivationLagThreshold)
	}

	if kafka.TLS != nil {
		triggerMeta["tls"] = "enable"
	}

	if kafka.SASL != nil && kafka.SASL.Mechanism != "" {
		triggerMeta["sasl"] = kedaSASLType(kafka.SASL.Mechanism)
	}

	triggerSpec := map[string]interface{}{
		"type":     "kafka",
		"metadata": triggerMeta,
	}

	if needsTriggerAuth(kafka) {
		triggerSpec["authenticationRef"] = map[string]interface{}{
			"name": triggerAuthName(f.Name),
		}
	}

	so := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata": map[string]interface{}{
				"name":      scaledObjectName(f.Name),
				"namespace": namespace,
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "apps/v1",
						"kind":               "Deployment",
						"name":               deployment.Name,
						"uid":                string(deployment.UID),
						"controller":         true,
						"blockOwnerDeletion": true,
					},
				},
			},
			"spec": map[string]interface{}{
				"scaleTargetRef": map[string]interface{}{
					"kind": "Deployment",
					"name": deployment.Name,
				},
				"minReplicaCount": int64(minScale),
				"maxReplicaCount": int64(maxScale),
				"cooldownPeriod":  int64(300),
				"triggers": []interface{}{
					triggerSpec,
				},
			},
		},
	}

	return so
}

// ensureScaledObject creates or updates a KEDA ScaledObject for Kafka scaling.
func ensureScaledObject(ctx context.Context, dynClient dynamic.Interface, so *unstructured.Unstructured) error {
	ns := so.GetNamespace()
	name := so.GetName()
	client := dynClient.Resource(scaledObjectGVR).Namespace(ns)

	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := client.Create(ctx, so, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create ScaledObject %s/%s: %w", ns, name, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get ScaledObject %s/%s: %w", ns, name, err)
	}

	so.SetResourceVersion(existing.GetResourceVersion())
	if _, err := client.Update(ctx, so, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update ScaledObject %s/%s: %w", ns, name, err)
	}
	return nil
}

// ensureTriggerAuth creates or updates a KEDA TriggerAuthentication.
func ensureTriggerAuth(ctx context.Context, dynClient dynamic.Interface, ta *unstructured.Unstructured) error {
	ns := ta.GetNamespace()
	name := ta.GetName()
	client := dynClient.Resource(triggerAuthGVR).Namespace(ns)

	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if _, err := client.Create(ctx, ta, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create TriggerAuthentication %s/%s: %w", ns, name, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get TriggerAuthentication %s/%s: %w", ns, name, err)
	}

	ta.SetResourceVersion(existing.GetResourceVersion())
	if _, err := client.Update(ctx, ta, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update TriggerAuthentication %s/%s: %w", ns, name, err)
	}
	return nil
}

// deleteScaledObject removes a ScaledObject if it exists.
func deleteScaledObject(ctx context.Context, dynClient dynamic.Interface, ns, name string) error {
	client := dynClient.Resource(scaledObjectGVR).Namespace(ns)
	err := client.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ScaledObject %s/%s: %w", ns, name, err)
	}
	return nil
}

// deleteTriggerAuth removes a TriggerAuthentication if it exists.
func deleteTriggerAuth(ctx context.Context, dynClient dynamic.Interface, ns, name string) error {
	client := dynClient.Resource(triggerAuthGVR).Namespace(ns)
	err := client.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete TriggerAuthentication %s/%s: %w", ns, name, err)
	}
	return nil
}
