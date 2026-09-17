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
	"k8s.io/apimachinery/pkg/util/validation"
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

// validateKafkaResourceNames refuses a function whose Kafka scaler resource
// names would not be valid DNS-1035 labels. buildScaledObject names the
// ScaledObject <name>-kafka and, when credentials are configured,
// buildTriggerAuth names the TriggerAuthentication <name>-kafka-auth -- suffixes
// that push a ~63-character function name past the 63-character limit, so the
// API server rejects the resource for an otherwise valid function. This is the
// Kafka-path counterpart to validateBridgeName on the HTTP path; needAuth
// selects the longest suffix that will actually be created.
func validateKafkaResourceNames(name string, needAuth bool) error {
	// triggerAuthName's suffix is the longer of the two, so when auth is
	// configured its name is the binding constraint; otherwise only the
	// ScaledObject is created.
	longest := scaledObjectName(name)
	if needAuth {
		longest = triggerAuthName(name)
	}
	if errs := validation.IsDNS1035Label(longest); len(errs) > 0 {
		return fmt.Errorf(
			"function name %q is too long for the keda deployer: its Kafka scaler resource would be named %q, which is not a valid name (%s)",
			name, longest, strings.Join(errs, "; "))
	}
	return nil
}

// triggers returns the function's explicitly configured KEDA triggers, or a
// plain http trigger when none are configured at all. This only matters for
// callers of Deploy that bypass fn.Function.Validate (which requires
// deployer: keda to declare triggers explicitly) -- e.g. tests and other
// direct API consumers. It never infers a kafka trigger: that decision is
// never made silently, on any path.
func triggers(f fn.Function) []fn.KEDATrigger {
	if f.Scale != nil && f.Scale.KEDA != nil {
		return f.Scale.KEDA.Triggers
	}
	if f.Scale != nil && f.Scale.KPA != nil {
		// scale.kpa is incompatible with deployer: keda (ValidateScale
		// rejects it), but Deploy is reachable without validation first:
		// falling back to the default http trigger here would silently
		// treat this as "no scale.keda configured" instead of surfacing
		// the mismatch. An empty trigger list makes Deploy's existing
		// empty-triggers guard reject it instead.
		return nil
	}
	return []fn.KEDATrigger{{Type: "http"}}
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
	if kafka.TLS != nil && (kafka.TLS.CACert != "" || kafka.TLS.ClientCert != "" || kafka.TLS.ClientKey != "") {
		return true
	}
	return false
}

// parseSecretRef extracts the secret name and key from a {{ secret:name:key }}
// reference. Returns empty strings if the value is not a secret reference.
// Uses fn.TemplateRefPattern -- the same pattern fn.Function.Validate and
// pkg/k8s/deployer.go's env wiring match against -- so a malformed
// {{ ... }} value is treated the same way everywhere instead of being
// accepted here via ad-hoc trim/split and looked up as a mismatched secret.
func parseSecretRef(value string) (secretName, secretKey string) {
	matches := fn.TemplateRefPattern.FindStringSubmatch(value)
	if matches == nil || matches[1] != "secret" {
		return
	}
	return matches[2], matches[3]
}

// findSecretForPath finds the volume secret name that backs a given file path.
// It matches by checking which volume's mount path is a parent of the cert path.
func findSecretForPath(certPath string, volumes []fn.Volume) (secretName, key string) {
	for _, v := range volumes {
		if v.Secret == nil || v.Path == nil {
			continue
		}
		mountPath := filepath.Clean(*v.Path)
		cp := filepath.Clean(certPath)
		if cp == mountPath || strings.HasPrefix(cp, mountPath+string(filepath.Separator)) {
			rel, err := filepath.Rel(mountPath, cp)
			if err != nil {
				continue
			}
			// func mounts a Secret volume at a single directory level, so its
			// data keys are plain filenames: certPath == mountPath (rel == ".")
			// names the directory, not a file in it, and a rel containing a
			// separator names a file nested more than one level deep. Neither
			// is a valid Secret data key.
			if rel == "." || strings.ContainsRune(rel, filepath.Separator) {
				continue
			}
			return *v.Secret, rel
		}
	}
	return
}

// validateKafkaTLSPaths checks that any explicitly-configured
// run.kafka.tls path (caCert/clientCert/clientKey) resolves to a
// configured volume. Deliberately depends only on kafka/volumes -- no
// live deployment object -- so it can run as a Deploy preflight guard,
// before the raw Deployment/Service exist, and not just inside
// buildTriggerAuth once one does.
func validateKafkaTLSPaths(kafka *fn.KafkaConfig, volumes []fn.Volume) error {
	if kafka == nil || kafka.TLS == nil {
		return nil
	}
	if kafka.TLS.CACert != "" {
		if name, _ := findSecretForPath(kafka.TLS.CACert, volumes); name == "" {
			return fmt.Errorf("run.kafka.tls.caCert %q does not match any configured volume", kafka.TLS.CACert)
		}
	}
	if kafka.TLS.ClientCert != "" {
		if name, _ := findSecretForPath(kafka.TLS.ClientCert, volumes); name == "" {
			return fmt.Errorf("run.kafka.tls.clientCert %q does not match any configured volume", kafka.TLS.ClientCert)
		}
	}
	if kafka.TLS.ClientKey != "" {
		if name, _ := findSecretForPath(kafka.TLS.ClientKey, volumes); name == "" {
			return fmt.Errorf("run.kafka.tls.clientKey %q does not match any configured volume", kafka.TLS.ClientKey)
		}
	}
	return nil
}

// buildTriggerAuth creates the unstructured TriggerAuthentication for Kafka
// SASL/TLS. Returns a non-nil error when a TLS path is explicitly
// configured but doesn't resolve to any configured volume -- distinct from
// a field that was never set at all, which is silently skipped.
func buildTriggerAuth(f fn.Function, deployment *v1.Deployment, namespace string) (*unstructured.Unstructured, error) {
	kafka := f.Run.Kafka
	if kafka == nil {
		return nil, nil
	}
	if err := validateKafkaTLSPaths(kafka, f.Run.Volumes); err != nil {
		return nil, err
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
		} else {
			// Plaintext value: ends up as a literal KAFKA_SASL_PASSWORD env var
			// on the function's container. A {{ configMap:... }} reference
			// ends up as a KAFKA_SASL_PASSWORD env var too, but backed by a
			// ConfigMapKeyRef instead of a literal value (see
			// appendKafkaEnvValue in pkg/k8s/deployer.go). Either way the env
			// var name is the same, so point KEDA at that name instead of a
			// secretTargetRef. Plaintext is allowed here, at least for
			// debugging purposes -- func doesn't force SASL credentials
			// through Secrets.
			envRefs = append(envRefs, map[string]interface{}{
				"parameter":     "password",
				"name":          "KAFKA_SASL_PASSWORD",
				"containerName": deployment.Spec.Template.Spec.Containers[0].Name,
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
		if caSecretName == "" {
			return nil, fmt.Errorf("run.kafka.tls.caCert %q does not match any configured volume", kafka.TLS.CACert)
		}
		secretRefs = append(secretRefs, map[string]interface{}{
			"parameter": "ca",
			"name":      caSecretName,
			"key":       caKey,
		})
	}

	if kafka.TLS != nil && kafka.TLS.ClientCert != "" {
		certSecretName, certKey := findSecretForPath(kafka.TLS.ClientCert, f.Run.Volumes)
		if certSecretName == "" {
			return nil, fmt.Errorf("run.kafka.tls.clientCert %q does not match any configured volume", kafka.TLS.ClientCert)
		}
		secretRefs = append(secretRefs, map[string]interface{}{
			"parameter": "cert",
			"name":      certSecretName,
			"key":       certKey,
		})
	}

	if kafka.TLS != nil && kafka.TLS.ClientKey != "" {
		keySecretName, keyKey := findSecretForPath(kafka.TLS.ClientKey, f.Run.Volumes)
		if keySecretName == "" {
			return nil, fmt.Errorf("run.kafka.tls.clientKey %q does not match any configured volume", kafka.TLS.ClientKey)
		}
		secretRefs = append(secretRefs, map[string]interface{}{
			"parameter": "key",
			"name":      keySecretName,
			"key":       keyKey,
		})
	}

	if len(secretRefs) == 0 && len(envRefs) == 0 {
		return nil, nil
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
						// blockOwnerDeletion deliberately omitted: it only
						// takes effect for foreground cascading deletion
						// (unused here), but makes the
						// OwnerReferencesPermissionEnforcement admission
						// plugin require update on the owner's finalizers
						// subresource -- a grant the deploying/pipeline
						// service account doesn't hold by default, so the
						// create is rejected outright on OpenShift, which
						// enables that plugin (KinD doesn't). Same reasoning
						// as the Service owner reference in
						// pkg/k8s/deployer.go.
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       deployment.Name,
						"uid":        string(deployment.UID),
						"controller": true,
					},
				},
			},
			"spec": spec,
		},
	}

	return ta, nil
}

// kedaSASLType maps func.yaml SASL mechanism names to KEDA trigger metadata values.
func kedaSASLType(mechanism string) string {
	switch mechanism {
	case "SCRAM-SHA-256":
		return "scram_sha256"
	case "SCRAM-SHA-512":
		return "scram_sha512"
	case "PLAIN":
		return "plaintext"
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

	if kafka.SecurityProtocol == "SSL" || kafka.SecurityProtocol == "SASL_SSL" {
		// Enable KEDA's TLS handshake based on securityProtocol, not the
		// presence of run.kafka.tls: a valid config can set SSL/SASL_SSL and
		// rely on the system's CA trust store, with no explicit tls block at
		// all. Gating on kafka.TLS != nil would leave KEDA attempting a
		// plaintext connection for that config.
		triggerMeta["tls"] = "enable"

		if kafka.TLS != nil && kafka.TLS.SkipVerify {
			// SkipVerify is propagated to the function's own container (see
			// pkg/functions/runner.go), but KEDA's scaler connects to the
			// broker independently -- without this, a self-signed broker
			// lets the app's consumer connect while the scaler's own TLS
			// handshake fails and the ScaledObject never scales, silently.
			triggerMeta["unsafeSsl"] = "true"
		}
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
						// blockOwnerDeletion deliberately omitted: it only
						// takes effect for foreground cascading deletion
						// (unused here), but makes the
						// OwnerReferencesPermissionEnforcement admission
						// plugin require update on the owner's finalizers
						// subresource -- a grant the deploying/pipeline
						// service account doesn't hold by default, so the
						// create is rejected outright on OpenShift, which
						// enables that plugin (KinD doesn't). Same reasoning
						// as the Service owner reference in
						// pkg/k8s/deployer.go.
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       deployment.Name,
						"uid":        string(deployment.UID),
						"controller": true,
					},
				},
			},
			"spec": buildScaledObjectSpec(f, deployment.Name, minScale, maxScale, triggerSpec),
		},
	}

	return so
}

func buildScaledObjectSpec(f fn.Function, deploymentName string, minScale, maxScale int32, triggerSpec map[string]interface{}) map[string]interface{} {
	cooldown := int64(300)
	polling := int64(30)
	if f.Scale != nil && f.Scale.KEDA != nil {
		if f.Scale.KEDA.CooldownPeriod != nil {
			cooldown = int64(*f.Scale.KEDA.CooldownPeriod)
		}
		if f.Scale.KEDA.PollingInterval != nil {
			polling = int64(*f.Scale.KEDA.PollingInterval)
		}
	}

	return map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"kind": "Deployment",
			"name": deploymentName,
		},
		"minReplicaCount": int64(minScale),
		"maxReplicaCount": int64(maxScale),
		"cooldownPeriod":  cooldown,
		"pollingInterval": polling,
		"triggers": []interface{}{
			triggerSpec,
		},
	}
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
	// KEDA attaches its own finalizer to a ScaledObject it's managing; this
	// Update is a full replace, so without carrying it over, a redeploy
	// would silently strip it and let a later delete bypass KEDA's cleanup
	// ordering (see the note on this in kafka_scaling_int_test.go).
	so.SetFinalizers(existing.GetFinalizers())
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
	// Same reasoning as ensureScaledObject: preserve KEDA's own finalizer
	// across this full-replace Update instead of silently dropping it.
	ta.SetFinalizers(existing.GetFinalizers())
	if _, err := client.Update(ctx, ta, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update TriggerAuthentication %s/%s: %w", ns, name, err)
	}
	return nil
}

// deleteScaledObject removes a ScaledObject if it exists. A nil return
// means the delete was accepted, not that the object is actually gone yet:
// KEDA attaches its own finalizer to this resource, so removal completes
// asynchronously once KEDA's controller processes it. Callers that
// immediately create a replacement scaler accept this as a rare,
// self-correcting race rather than polling for actual removal here.
func deleteScaledObject(ctx context.Context, dynClient dynamic.Interface, ns, name string) error {
	client := dynClient.Resource(scaledObjectGVR).Namespace(ns)
	err := client.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ScaledObject %s/%s: %w", ns, name, err)
	}
	return nil
}

// deleteTriggerAuth removes a TriggerAuthentication if it exists. Same
// finalizer/async-removal caveat as deleteScaledObject.
func deleteTriggerAuth(ctx context.Context, dynClient dynamic.Interface, ns, name string) error {
	client := dynClient.Resource(triggerAuthGVR).Namespace(ns)
	err := client.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete TriggerAuthentication %s/%s: %w", ns, name, err)
	}
	return nil
}
