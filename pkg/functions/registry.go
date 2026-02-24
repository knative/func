package functions

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// CredentialsCallback is a function that returns credentials for a registry image.
// It is defined here to avoid importing concrete credential packages.
type CredentialsCallback func(ctx context.Context, image string) (username, password string, err error)

// BuilderConfig holds all parameters needed by a builder factory.
// Concrete builder packages receive this struct and convert it to their own types.
type BuilderConfig struct {
	// Verbose enables diagnostic output.
	Verbose bool
	// Transport is the HTTP transport used for registry communication.
	Transport http.RoundTripper
	// RegistryInsecure disables TLS certificate verification.
	RegistryInsecure bool
	// Credentials provides registry authentication.
	Credentials CredentialsCallback
	// WithTimestamp stamps the built image with the current time (Pack builder only).
	WithTimestamp bool
}

// DeployDecorator allows customizing deployment metadata (annotations and labels)
// per-deployment. It is defined here — without importing pkg/deployer — so that
// DeployerConfig can carry one without creating an import cycle.
type DeployDecorator interface {
	UpdateAnnotations(Function, map[string]string) map[string]string
	UpdateLabels(Function, map[string]string) map[string]string
}

// DeployerConfig holds all parameters needed by a deployer factory.
type DeployerConfig struct {
	// Verbose enables diagnostic output.
	Verbose bool

	// Decorator allows customizing deployment annotations and labels.
	// May be nil, in which case no decoration is applied.
	Decorator DeployDecorator
}

// BuilderFactory creates client options for a builder implementation.
// Each concrete builder package registers a factory that accepts BuilderConfig.
type BuilderFactory func(cfg BuilderConfig) []Option

// DeployerFactory creates client options for a deployer implementation.
// Each concrete deployer package registers a factory that accepts DeployerConfig.
type DeployerFactory func(cfg DeployerConfig) []Option

// BuilderRegistration holds a builder's factory and compatibility constraints.
type BuilderRegistration struct {
	// Name is the builder's short name (e.g. "pack", "s2i", "host", "wasm").
	Name string

	// Constraints are compatibility rules.
	// An empty slice means the builder accepts any runtime.
	Constraints []CompatibilityRule

	// Factory creates the client options needed to use this builder.
	Factory BuilderFactory
}

// DeployerRegistration holds a deployer's factory and compatibility constraints.
type DeployerRegistration struct {
	// Name is the deployer's short name (e.g. "knative", "raw", "keda", "wasm").
	Name string

	// Constraints are compatibility rules.
	// An empty slice means the deployer accepts any runtime.
	Constraints []CompatibilityRule

	// Factory creates the client options needed to use this deployer.
	Factory DeployerFactory
}

// SupportsRuntime reports whether all constraints agree the runtime is supported.
// An empty constraints list means all runtimes are accepted.
func (r *BuilderRegistration) SupportsRuntime(runtime string) bool {
	for _, c := range r.Constraints {
		if !c.SupportsRuntime(runtime) {
			return false
		}
	}
	return true
}

// SupportsRuntime reports whether all constraints agree the runtime is supported.
// An empty constraints list means all runtimes are accepted.
func (r *DeployerRegistration) SupportsRuntime(runtime string) bool {
	for _, c := range r.Constraints {
		if !c.SupportsRuntime(runtime) {
			return false
		}
	}
	return true
}

// PostProcessor transforms the CompatibilityRules of registrations whose name
// satisfies Matches.  Post-processors are applied lazily the first time a
// registration is resolved.
//
// The Process function receives the current list of constraints and returns the
// updated list; it may append, replace, or remove entries.
type PostProcessor struct {
	// Matches decides whether this post-processor applies to a given registration name.
	Matches RuntimeMatcher
	// Process transforms the existing constraints and returns the updated slice.
	Process func(rules []CompatibilityRule) ([]CompatibilityRule, error)
}

// Registry holds all registered builders and deployers.
// Create one with NewRegistry() and pass it to each package's Register() function.
type Registry struct {
	mu sync.Mutex

	builders          map[string]*BuilderRegistration
	buildersProcessed map[string]bool
	builderPPs        []PostProcessor

	deployers          map[string]*DeployerRegistration
	deployersProcessed map[string]bool
	deployerPPs        []PostProcessor
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		builders:           make(map[string]*BuilderRegistration),
		buildersProcessed:  make(map[string]bool),
		deployers:          make(map[string]*DeployerRegistration),
		deployersProcessed: make(map[string]bool),
	}
}

// RegisterBuilder registers a builder with its factory and optional constraints.
// Pass no constraints to accept every runtime.
func (r *Registry) RegisterBuilder(name string, factory BuilderFactory, constraints ...CompatibilityRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[name] = &BuilderRegistration{
		Name:        name,
		Factory:     factory,
		Constraints: constraints,
	}
	// Invalidate so post-processors are (re-)applied on next access.
	delete(r.buildersProcessed, name)
}

// RegisterDeployer registers a deployer with its factory and optional constraints.
// Pass no constraints to accept every runtime.
func (r *Registry) RegisterDeployer(name string, factory DeployerFactory, constraints ...CompatibilityRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deployers[name] = &DeployerRegistration{
		Name:        name,
		Factory:     factory,
		Constraints: constraints,
	}
	delete(r.deployersProcessed, name)
}

// RegisterBuilderPostProcessor stores a post-processor that will be applied lazily
// to every builder whose name satisfies pp.Matches on the next access.
// Adding a post-processor invalidates the processed cache so existing registrations
// are re-evaluated on the next query.
func (r *Registry) RegisterBuilderPostProcessor(pp PostProcessor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builderPPs = append(r.builderPPs, pp)
	// Invalidate so all builders pick up the new post-processor.
	r.buildersProcessed = make(map[string]bool)
}

// RegisterDeployerPostProcessor stores a post-processor applied lazily to deployers.
func (r *Registry) RegisterDeployerPostProcessor(pp PostProcessor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deployerPPs = append(r.deployerPPs, pp)
	r.deployersProcessed = make(map[string]bool)
}

// GetBuilder returns the registration for the named builder, with all
// post-processors applied.
func (r *Registry) GetBuilder(name string) (*BuilderRegistration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.builders[name]
	if !ok {
		return nil, false
	}
	if !r.buildersProcessed[name] {
		r.applyBuilderPPs(reg)
		r.buildersProcessed[name] = true
	}
	return reg, true
}

// GetDeployer returns the registration for the named deployer, with all
// post-processors applied.
func (r *Registry) GetDeployer(name string) (*DeployerRegistration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.deployers[name]
	if !ok {
		return nil, false
	}
	if !r.deployersProcessed[name] {
		r.applyDeployerPPs(reg)
		r.deployersProcessed[name] = true
	}
	return reg, true
}

// ListBuilders returns all registered builder names.
func (r *Registry) ListBuilders() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	return names
}

// ListDeployers returns all registered deployer names.
func (r *Registry) ListDeployers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.deployers))
	for name := range r.deployers {
		names = append(names, name)
	}
	return names
}

// InferBuilder returns the registered builder name that has explicit constraints
// and supports the given runtime, or an empty string if no such builder exists.
//
// A registration is considered "inferred" only when it was registered with at
// least one explicit CompatibilityRule (i.e., its Constraints slice is non-empty
// before post-processors run).  Registrations with no constraints accept every
// runtime by default and are never returned by inference — they represent the
// conventional defaults (e.g. pack, s2i, host) that the caller selects explicitly
// or falls back to via its own default logic.
//
// Post-processors are applied lazily as usual.
func (r *Registry) InferBuilder(runtime string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, reg := range r.builders {
		if len(reg.Constraints) == 0 {
			continue // unconstrained: conventional default, not inferred
		}
		if !r.buildersProcessed[name] {
			r.applyBuilderPPs(reg)
			r.buildersProcessed[name] = true
		}
		if reg.SupportsRuntime(runtime) {
			return name
		}
	}
	return ""
}

// InferDeployer returns the registered deployer name that has explicit constraints
// and supports the given runtime, or an empty string if no such deployer exists.
//
// See InferBuilder for the semantics of inference vs. default selection.
func (r *Registry) InferDeployer(runtime string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, reg := range r.deployers {
		if len(reg.Constraints) == 0 {
			continue // unconstrained: conventional default, not inferred
		}
		if !r.deployersProcessed[name] {
			r.applyDeployerPPs(reg)
			r.deployersProcessed[name] = true
		}
		if reg.SupportsRuntime(runtime) {
			return name
		}
	}
	return ""
}

// ValidateBuilderCompatibility returns an error if the named builder does not
// support the given runtime, or if the builder is unknown.
// Incompatibility errors wrap ErrIncompatibility so callers can use errors.Is.
func (r *Registry) ValidateBuilderCompatibility(runtime, builder string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.builders[builder]
	if !ok {
		return fmt.Errorf("unknown builder: %q", builder)
	}
	if !r.buildersProcessed[builder] {
		r.applyBuilderPPs(reg)
		r.buildersProcessed[builder] = true
	}
	if !reg.SupportsRuntime(runtime) {
		return fmt.Errorf("builder %q does not support runtime %q: %w", builder, runtime, ErrIncompatibility)
	}
	return nil
}

// ValidateDeployerCompatibility returns an error if the named deployer does not
// support the given runtime, or if the deployer is unknown.
// Incompatibility errors wrap ErrIncompatibility so callers can use errors.Is.
func (r *Registry) ValidateDeployerCompatibility(runtime, deployer string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.deployers[deployer]
	if !ok {
		return fmt.Errorf("unknown deployer: %q", deployer)
	}
	if !r.deployersProcessed[deployer] {
		r.applyDeployerPPs(reg)
		r.deployersProcessed[deployer] = true
	}
	if !reg.SupportsRuntime(runtime) {
		return fmt.Errorf("deployer %q does not support runtime %q: %w", deployer, runtime, ErrIncompatibility)
	}
	return nil
}

// --- internal helpers (called with r.mu already held) ---

func (r *Registry) applyBuilderPPs(reg *BuilderRegistration) {
	for _, pp := range r.builderPPs {
		if !pp.Matches(reg.Name) {
			continue
		}
		updated, err := pp.Process(reg.Constraints)
		if err == nil {
			reg.Constraints = updated
		}
	}
}

func (r *Registry) applyDeployerPPs(reg *DeployerRegistration) {
	for _, pp := range r.deployerPPs {
		if !pp.Matches(reg.Name) {
			continue
		}
		updated, err := pp.Process(reg.Constraints)
		if err == nil {
			reg.Constraints = updated
		}
	}
}
