package functions

import "strconv"

// KafkaRuntimeEnv is one environment variable configuring the Kafka runtime
// (knative.dev/func-adapter-kafka) from run.kafka.
//
// KafkaConnectionEnvs and KafkaDeliveryEnvs are the single mapping from
// run.kafka to the runtime's environment. Both the sidecar the cluster
// deployers inject (k8s.AppendKafkaEnvs, k8s.AppendKafkaDeliveryEnvs) and the
// runtime `func run` starts locally are configured from them, so the two always
// set the same variables: the names are func's side of its contract with the
// runtime, which reads exactly these names.
type KafkaRuntimeEnv struct {
	Name  string
	Value string

	// Field is set to the run.kafka field a credential comes from
	// (run.kafka.sasl.user or run.kafka.sasl.password). Its Value may be a
	// {{ secret:name:key }} or {{ configMap:name:key }} reference, which each
	// consumer resolves its own way: the cluster deployers into an env var
	// source, a local run as run.envs references are resolved. Field is empty
	// for every other variable, whose Value is used as is.
	Field string
}

// KafkaConnectionEnvs returns the runtime's connection and security
// configuration: brokers, topic and consumer group, then any security
// protocol, TLS and SASL settings. A nil or incomplete k (one missing brokers,
// topic or consumer group) yields none. Optional settings left unset are
// omitted, except that a TLS block always sets KAFKA_TLS_SKIP_VERIFY and a
// SASL block always sets KAFKA_SASL_MECHANISM (see KafkaSASL.EffectiveMechanism).
func KafkaConnectionEnvs(k *KafkaConfig) []KafkaRuntimeEnv {
	if k == nil || k.Brokers == "" || k.Topic == "" || k.ConsumerGroup == "" {
		return nil
	}
	envs := []KafkaRuntimeEnv{
		{Name: "KAFKA_BROKERS", Value: k.Brokers},
		{Name: "KAFKA_TOPIC", Value: k.Topic},
		{Name: "KAFKA_CONSUMER_GROUP", Value: k.ConsumerGroup},
	}
	if k.SecurityProtocol != "" {
		envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_SECURITY_PROTOCOL", Value: k.SecurityProtocol})
	}
	if t := k.TLS; t != nil {
		if t.CACert != "" {
			envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_TLS_CA_CERT", Value: t.CACert})
		}
		if t.ClientCert != "" {
			envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_TLS_CLIENT_CERT", Value: t.ClientCert})
		}
		if t.ClientKey != "" {
			envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_TLS_CLIENT_KEY", Value: t.ClientKey})
		}
		envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_TLS_SKIP_VERIFY", Value: strconv.FormatBool(t.SkipVerify)})
	}
	if s := k.SASL; s != nil {
		envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_SASL_MECHANISM", Value: s.EffectiveMechanism()})
		if s.User != "" {
			envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_SASL_USER", Value: s.User, Field: "run.kafka.sasl.user"})
		}
		if s.Password != "" {
			envs = append(envs, KafkaRuntimeEnv{Name: "KAFKA_SASL_PASSWORD", Value: s.Password, Field: "run.kafka.sasl.password"})
		}
	}
	return envs
}

// KafkaDeliveryEnvs returns the runtime's delivery tuning: how a consumed
// record is delivered to the function. Each variable maps 1:1 to a
// run.kafka.delivery field; an unset field is omitted so the runtime applies
// its own default. A nil k or nil delivery yields none.
func KafkaDeliveryEnvs(k *KafkaConfig) []KafkaRuntimeEnv {
	if k == nil || k.Delivery == nil {
		return nil
	}
	d := k.Delivery
	var envs []KafkaRuntimeEnv
	// retry is a pointer: nil means "omit, retry without limit", while 0 is an
	// explicit single-attempt budget that must be passed on.
	if d.Retry != nil {
		envs = append(envs, KafkaRuntimeEnv{Name: "FUNCTION_DELIVERY_RETRY", Value: strconv.Itoa(*d.Retry)})
	}
	if d.BackoffPolicy != "" {
		envs = append(envs, KafkaRuntimeEnv{Name: "FUNCTION_DELIVERY_BACKOFF_POLICY", Value: d.BackoffPolicy})
	}
	if d.BackoffDelay != "" {
		envs = append(envs, KafkaRuntimeEnv{Name: "FUNCTION_DELIVERY_BACKOFF_DELAY", Value: d.BackoffDelay})
	}
	if d.BackoffMaxDelay != "" {
		envs = append(envs, KafkaRuntimeEnv{Name: "FUNCTION_DELIVERY_BACKOFF_MAX_DELAY", Value: d.BackoffMaxDelay})
	}
	// timeout maps to the runtime's per-attempt request timeout.
	if d.Timeout != "" {
		envs = append(envs, KafkaRuntimeEnv{Name: "FUNCTION_REQUEST_TIMEOUT", Value: d.Timeout})
	}
	return envs
}
