package function

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const (
	DefaultRegistry = "docker.io"
	DefaultRuntime  = "node"
	DefaultTrigger  = "http"
)

// Client for managing Function instances.
type Client struct {
	verbose          bool     // print verbose logs
	builder          Builder  // Builds a runnable image from Function source
	pusher           Pusher   // Pushes the image assocaited with a Function.
	deployer         Deployer // Deploys or Updates a Function
	runner           Runner   // Runs the Function locally
	remover          Remover  // Removes remote services
	lister           Lister   // Lists remote services
	describer        Describer
	dnsProvider      DNSProvider      // Provider of DNS services
	templates        string           // path to extensible templates
	registry         string           // default registry for OCI image tags
	progressListener ProgressListener // progress listener
	emitter          Emitter          // Emits CloudEvents to functions
}

// ErrNotBuilt indicates the Function has not yet been built.
var ErrNotBuilt = errors.New("not built")

// Builder of Function source to runnable image.
type Builder interface {
	// Build a Function project with source located at path.
	Build(context.Context, Function) error
}

// Pusher of Function image to a registry.
type Pusher interface {
	// Push the image of the Function.
	// Returns Image Digest - SHA256 hash of the produced image
	Push(ctx context.Context, f Function) (string, error)
}

type Status int

const (
	Failed Status = iota
	Deployed
	Updated
)

type DeploymentResult struct {
	Status Status
	URL    string
}

// Deployer of Function source to running status.
type Deployer interface {
	// Deploy a Function of given name, using given backing image.
	Deploy(context.Context, Function) (DeploymentResult, error)
}

// Runner runs the Function locally.
type Runner interface {
	// Run the Function locally.
	Run(context.Context, Function) error
}

// Remover of deployed services.
type Remover interface {
	// Remove the Function from remote.
	Remove(ctx context.Context, name string) error
}

// Lister of deployed services.
type Lister interface {
	// List the Functions currently deployed.
	List(ctx context.Context) ([]ListItem, error)
}

type ListItem struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Runtime   string `json:"runtime" yaml:"runtime"`
	URL       string `json:"url" yaml:"url"`
	Ready     string `json:"ready" yaml:"ready"`
}

// ProgressListener is notified of task progress.
type ProgressListener interface {
	// SetTotal steps of the given task.
	SetTotal(int)

	// Increment to the next step with the given message.
	Increment(message string)

	// Complete signals completion, which is expected to be somewhat different than a step increment.
	Complete(message string)

	// Done signals a cessation of progress updates.  Should be called in a defer statement to ensure
	// the progress listener can stop any outstanding tasks such as synchronous user updates.
	Done()
}

// Describer of Functions' remote deployed aspect.
type Describer interface {
	// Describe the running state of the service as reported by the underlyng platform.
	Describe(name string) (description Description, err error)
}

type Description struct {
	Name          string         `json:"name" yaml:"name"`
	Image         string         `json:"image" yaml:"image"`
	Namespace     string         `json:"namespace" yaml:"namespace"`
	Routes        []string       `json:"routes" yaml:"routes"`
	Subscriptions []Subscription `json:"subscriptions" yaml:"subscriptions"`
}

type Subscription struct {
	Source string `json:"source" yaml:"source"`
	Type   string `json:"type" yaml:"type"`
	Broker string `json:"broker" yaml:"broker"`
}

// DNSProvider exposes DNS services necessary for serving the Function.
type DNSProvider interface {
	// Provide the given name by routing requests to address.
	Provide(Function) error
}

// Emit CloudEvents to functions
type Emitter interface {
	Emit(ctx context.Context, endpoint string) error
}

// New client for Function management.
func New(options ...Option) *Client {
	// Instantiate client with static defaults.
	c := &Client{
		builder:          &noopBuilder{output: os.Stdout},
		pusher:           &noopPusher{output: os.Stdout},
		deployer:         &noopDeployer{output: os.Stdout},
		runner:           &noopRunner{output: os.Stdout},
		remover:          &noopRemover{output: os.Stdout},
		lister:           &noopLister{output: os.Stdout},
		dnsProvider:      &noopDNSProvider{output: os.Stdout},
		progressListener: &noopProgressListener{},
		emitter:          &noopEmitter{},
	}

	// Apply passed options, which take ultimate precidence.
	for _, o := range options {
		o(c)
	}
	return c
}

// Option defines a Function which when passed to the Client constructor optionally
// mutates private members at time of instantiation.
type Option func(*Client)

// WithVerbose toggles verbose logging.
func WithVerbose(v bool) Option {
	return func(c *Client) {
		c.verbose = v
	}
}

// WithBuilder provides the concrete implementation of a builder.
func WithBuilder(d Builder) Option {
	return func(c *Client) {
		c.builder = d
	}
}

// WithPusher provides the concrete implementation of a pusher.
func WithPusher(d Pusher) Option {
	return func(c *Client) {
		c.pusher = d
	}
}

// WithDeployer provides the concrete implementation of a deployer.
func WithDeployer(d Deployer) Option {
	return func(c *Client) {
		c.deployer = d
	}
}

// WithRunner provides the concrete implementation of a deployer.
func WithRunner(r Runner) Option {
	return func(c *Client) {
		c.runner = r
	}
}

// WithRemover provides the concrete implementation of a remover.
func WithRemover(r Remover) Option {
	return func(c *Client) {
		c.remover = r
	}
}

// WithLister provides the concrete implementation of a lister.
func WithLister(l Lister) Option {
	return func(c *Client) {
		c.lister = l
	}
}

// WithDescriber provides a concrete implementation of a Function describer.
func WithDescriber(describer Describer) Option {
	return func(c *Client) {
		c.describer = describer
	}
}

// WithProgressListener provides a concrete implementation of a listener to
// be notified of progress updates.
func WithProgressListener(p ProgressListener) Option {
	return func(c *Client) {
		c.progressListener = p
	}
}

// WithDNSProvider proivdes a DNS provider implementation for registering the
// effective DNS name which is either explicitly set via WithName or is derived
// from the root path.
func WithDNSProvider(provider DNSProvider) Option {
	return func(c *Client) {
		c.dnsProvider = provider
	}
}

// WithTemplates sets the location to use for extensible templates.
// Extensible templates are additional templates that exist on disk and are
// not built into the binary.
func WithTemplates(templates string) Option {
	return func(c *Client) {
		c.templates = templates
	}
}

// WithRegistry sets the default registry which is consulted when an image name/tag
// is not explocitly provided.  Can be fully qualified, including the registry
// (ex: 'quay.io/myname') or simply the namespace 'myname' which indicates the
// the use of the default registry.
func WithRegistry(registry string) Option {
	return func(c *Client) {
		c.registry = registry
	}
}

// WithEmitter sets a CloudEvent emitter on the client which is capable of sending
// a CloudEvent to an arbitrary function endpoint
func WithEmitter(e Emitter) Option {
	return func(c *Client) {
		c.emitter = e
	}
}

// New Function.
// Use Create, Build and Deploy independently for lower level control.
func (c *Client) New(ctx context.Context, cfg Function) (err error) {
	c.progressListener.SetTotal(3)
	defer c.progressListener.Done()

	// Create local template
	err = c.Create(cfg)
	if err != nil {
		return
	}

	// Load the now-initialized Function.
	f, err := NewFunction(cfg.Root)
	if err != nil {
		return
	}

	// Build the now-initialized Function
	c.progressListener.Increment("Building container image")
	if err = c.Build(ctx, f.Root); err != nil {
		return
	}

	// Deploy the initialized Function, returning its publicly
	// addressible name for possible registration.
	c.progressListener.Increment("Deploying Function to cluster")
	if err = c.Deploy(ctx, f.Root); err != nil {
		return
	}

	// Create an external route to the Function
	c.progressListener.Increment("Creating route to Function")
	if err = c.Route(f.Root); err != nil {
		return
	}

	c.progressListener.Complete("Done")

	// TODO: use the knative client during deployment such that the actual final
	// route can be returned from the deployment step, passed to the DNS Router
	// for routing actual traffic, and returned here.
	if c.verbose {
		fmt.Printf("https://%v/\n", f.Name)
	}
	return
}

// Create a new Function project locally using the settings provided on a
// Function object.
func (c *Client) Create(cfg Function) (err error) {

	// Create project root directory, if it doesn't already exist
	if err = os.MkdirAll(cfg.Root, 0755); err != nil {
		return
	}

	// Create Function of the given root path.
	f, err := NewFunction(cfg.Root)
	if err != nil {
		return
	}

	// Assert the specified root is free of visible files and contentious
	// hidden files (the ConfigFile, which indicates it is already initialized)
	if err = assertEmptyRoot(f.Root); err != nil {
		return
	}

	// Map requested fields to the newly created function.
	f.Image = cfg.Image
	f.Name = cfg.Name

	// Assert runtime was provided, or default.
	f.Runtime = cfg.Runtime
	if f.Runtime == "" {
		f.Runtime = DefaultRuntime
	}

	// Assert trigger was provided, or default.
	f.Trigger = cfg.Trigger
	if f.Trigger == "" {
		f.Trigger = DefaultTrigger
	}

	// Write out a template.
	w := templateWriter{repositories: c.templates, verbose: c.verbose}
	if err = w.Write(f.Runtime, f.Trigger, f.Root); err != nil {
		return
	}

	// Check if template specifies a builder image. If so, add to configuration
	builderFilePath := filepath.Join(f.Root, ".builders.yaml")
	if builderConfig, err := os.ReadFile(builderFilePath); err == nil {
		// A .builder file was found. Read the default builder and set in the config file
		// TODO: A command line flag could be used to specify non-default builders
		builders := make(map[string]string)
		if err := yaml.Unmarshal(builderConfig, builders); err == nil {
			f.Builder = builders["default"]
			if c.verbose {
				fmt.Printf("Builder: %s\n", f.Builder)
			}
			f.BuilderMap = builders
		}
		// Remove the builders.yaml file so the user is not confused by a
		// configuration file that is only used for project creation/initialization
		if err := os.Remove(builderFilePath); err != nil {
			if c.verbose {
				fmt.Printf("Cannot remove %v. %v\n", builderFilePath, err)
			}
		}
	}

	// Write out the config.
	if err = writeConfig(f); err != nil {
		return
	}

	// TODO: Create a status structure and return it for clients to use
	// for output, such as from the CLI.
	if c.verbose {
		fmt.Println("Function project created")
	}
	return
}

// Build the Function at path.  Errors if the Function is either unloadable or does
// not contain a populated Image.
func (c *Client) Build(ctx context.Context, path string) (err error) {
	c.progressListener.Increment("Building function image")

	f, err := NewFunction(path)
	if err != nil {
		return
	}

	// Derive Image from the path (precedence is given to extant config)
	if f.Image, err = DerivedImage(path, c.registry); err != nil {
		return
	}

	if err = c.builder.Build(ctx, f); err != nil {
		return
	}

	// Write out config, which will now contain a populated image tag
	// if it had not already
	if err = writeConfig(f); err != nil {
		return
	}

	// TODO: create a statu structure and return it here for optional
	// use by the cli for user echo (rather than rely on verbose mode here)
	message := fmt.Sprintf("🙌 Function image built: %v", f.Image)
	c.progressListener.Increment(message)

	return
}

// Deploy the Function at path.  Errors if the Function has not been
// initialized with an image tag.
func (c *Client) Deploy(ctx context.Context, path string) (err error) {
	f, err := NewFunction(path)
	if err != nil {
		return
	}

	// Functions must be built (have an associated image) before being deployed.
	// Note that externally built images may be specified in the func.yaml
	if !f.Built() {
		return ErrNotBuilt
	}

	// Push the image for the named service to the configured registry
	c.progressListener.Increment("Pushing function image to the registry")
	imageDigest, err := c.pusher.Push(ctx, f)
	if err != nil {
		return
	}

	// Store the produced image Digest in the config
	f.ImageDigest = imageDigest
	if err = writeConfig(f); err != nil {
		return
	}

	// Deploy a new or Update the previously-deployed Function
	c.progressListener.Increment("Deploying function to the cluster")
	result, err := c.deployer.Deploy(ctx, f)
	if result.Status == Deployed {
		c.progressListener.Increment(fmt.Sprintf("Function deployed at URL: %v", result.URL))
	} else if result.Status == Updated {
		c.progressListener.Increment(fmt.Sprintf("Function updated at URL: %v", result.URL))
	}

	return err
}

func (c *Client) Route(path string) (err error) {
	// Ensure that the allocated final address is enabled with the
	// configured DNS provider.
	// NOTE:
	// DNS and TLS are provisioned by Knative Serving + cert-manager,
	// but DNS subdomain CNAME to the Kourier Load Balancer is
	// still manual, and the initial cluster config to suppot the TLD
	// is still manual.
	f, err := NewFunction(path)
	if err != nil {
		return
	}
	return c.dnsProvider.Provide(f)
}

// Run the Function whose code resides at root.
func (c *Client) Run(ctx context.Context, root string) error {

	// Create an instance of a Function representation at the given root.
	f, err := NewFunction(root)
	if err != nil {
		return err
	}

	if !f.Initialized() {
		// TODO: this needs a test.
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.  Please create one at this path in order to run.", root)
	}

	// delegate to concrete implementation of runner entirely.
	return c.runner.Run(ctx, f)
}

// List currently deployed Functions.
func (c *Client) List(ctx context.Context) ([]ListItem, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List(ctx)
}

// Describe a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used.
func (c *Client) Describe(name, root string) (d Description, err error) {
	// If name is provided, it takes precidence.
	// Otherwise load the Function defined at root.
	if name != "" {
		return c.describer.Describe(name)
	}

	f, err := NewFunction(root)
	if err != nil {
		return d, err
	}
	if !f.Initialized() {
		return d, fmt.Errorf("%v is not initialized", f.Name)
	}
	return c.describer.Describe(f.Name)
}

// Remove a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used if it exists.
func (c *Client) Remove(ctx context.Context, cfg Function) error {
	// If name is provided, it takes precidence.
	// Otherwise load the Function deined at root.
	if cfg.Name != "" {
		return c.remover.Remove(ctx, cfg.Name)
	}

	f, err := NewFunction(cfg.Root)
	if err != nil {
		return err
	}
	if !f.Initialized() {
		return fmt.Errorf("Function at %v can not be removed unless initialized.  Try removing by name.", f.Root)
	}
	return c.remover.Remove(ctx, f.Name)
}

// Emit a CloudEvent to a function endpoint
func (c *Client) Emit(ctx context.Context, endpoint string) error {
	return c.emitter.Emit(ctx, endpoint)
}

// Manual implementations (noops) of required interfaces.
// In practice, the user of this client package (for example the CLI) will
// provide a concrete implementation for all of the interfaces.  For testing or
// development, however, it is usefule that they are defaulted to noops and
// provded only when necessary.  Unit tests for the concrete implementations
// serve to keep the core logic here separate from the imperitive.
// -----------------------------------------------------

type noopBuilder struct{ output io.Writer }

func (n *noopBuilder) Build(ctx context.Context, _ Function) error { return nil }

type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(ctx context.Context, f Function) (string, error) { return "", nil }

type noopDeployer struct{ output io.Writer }

func (n *noopDeployer) Deploy(ctx context.Context, _ Function) (DeploymentResult, error) {
	return DeploymentResult{}, nil
}

type noopRunner struct{ output io.Writer }

func (n *noopRunner) Run(_ context.Context, _ Function) error { return nil }

type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(context.Context, string) error { return nil }

type noopLister struct{ output io.Writer }

func (n *noopLister) List(context.Context) ([]ListItem, error) { return []ListItem{}, nil }

type noopDNSProvider struct{ output io.Writer }

func (n *noopDNSProvider) Provide(_ Function) error { return nil }

type noopProgressListener struct{}

func (p *noopProgressListener) SetTotal(i int)     {}
func (p *noopProgressListener) Increment(m string) {}
func (p *noopProgressListener) Complete(m string)  {}
func (p *noopProgressListener) Done()              {}

type noopEmitter struct{}

func (p *noopEmitter) Emit(ctx context.Context, endpoint string) error { return nil }
