package wasm

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

// WasmBuilderRule returns the compatibility rule for the WASM builder.
func WasmBuilderRule() fn.CompatibilityRule {
	return fn.NewCompatibilityRule(
		fn.SuffixMatcher(WasiSuffix),
		"WASM builder supports WASI runtimes only (e.g., rust-wasi, go-wasi)",
	)
}

// WasmDeployerRule returns the compatibility rule for the WASM deployer.
func WasmDeployerRule() fn.CompatibilityRule {
	return fn.NewCompatibilityRule(
		fn.SuffixMatcher(WasiSuffix),
		"WASM deployer supports WASI runtimes only (e.g., rust-wasi, go-wasi)",
	)
}

// nonWasiConstraint builds a rule that rejects WASI runtimes with a clear
// message that names the entity (builder or deployer) responsible.
func nonWasiConstraint(entity string) fn.CompatibilityRule {
	return fn.NewCompatibilityRule(
		fn.NotMatcher(fn.SuffixMatcher(WasiSuffix)),
		entity+" does not support WASI runtimes; use the wasm "+entity+" instead",
	)
}

// nonWasiPostProcessor returns a PostProcessor that appends a nonWasiConstraint
// to every registration whose name is not exclusion (the wasm entry itself).
func nonWasiPostProcessor(exclusion, entity string) fn.PostProcessor {
	return fn.PostProcessor{
		Matches: fn.NotMatcher(fn.ExactMatcher(exclusion)),
		Process: func(rules []fn.CompatibilityRule) ([]fn.CompatibilityRule, error) {
			return append(rules, nonWasiConstraint(entity)), nil
		},
	}
}

// Register adds the WASM builder and deployer to the given registry, and
// installs post-processors that prevent traditional (non-wasm) builders and
// deployers from accepting WASI runtimes.
func Register(r *fn.Registry) {
	// Register WASM builder with WASI-only constraint.
	r.RegisterBuilder(BuilderName, wasmBuilderFactory, WasmBuilderRule())

	// Register WASM deployer with WASI-only constraint.
	r.RegisterDeployer(DeployerName, wasmDeployerFactory, WasmDeployerRule())

	// Post-processor: every builder that is NOT the wasm builder gets a
	// "non-WASI" constraint so that traditional builders clearly reject WASI runtimes.
	r.RegisterBuilderPostProcessor(nonWasiPostProcessor(BuilderName, "builder"))

	// Post-processor: every deployer that is NOT the wasm deployer gets the same.
	r.RegisterDeployerPostProcessor(nonWasiPostProcessor(DeployerName, "deployer"))
}

// wasmBuilderFactory creates the fn.Options needed to use the WASM builder.
func wasmBuilderFactory(cfg fn.BuilderConfig) []fn.Option {
	creds := adapterCredentials(cfg.Credentials)
	return []fn.Option{
		fn.WithBuilder(NewBuilder(
			WithVerbose(cfg.Verbose),
			WithCredentialsProvider(creds),
			WithTransport(cfg.Transport),
			WithInsecure(cfg.RegistryInsecure),
		)),
	}
}

// wasmDeployerFactory creates the fn.Options needed to use the WASM deployer.
// TODO: implement WasmModule CRD deployer (deploy to a WASM runtime on-cluster).
func wasmDeployerFactory(_ fn.DeployerConfig) []fn.Option {
	return nil
}

// adapterCredentials converts fn.CredentialsCallback to wasm.CredentialsProvider.
func adapterCredentials(cb fn.CredentialsCallback) CredentialsProvider {
	if cb == nil {
		return nil
	}
	return func(ctx context.Context, image string) (string, string, error) {
		return cb(ctx, image)
	}
}
