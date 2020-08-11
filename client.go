package faas

import (
	"fmt"
	"io"
	"os"
)

const DefaultNamespace = "faas"

// Client for a given Function.
type Client struct {
	verbose           bool        // print verbose logs
	local             bool        // Run in local-only mode
	internal          bool        // Deploy without publicly accessible route
	initializer       Initializer // Creates initial local function implementation
	builder           Builder     // Builds a runnable image from function source
	pusher            Pusher      // Pushes a built image to a registry
	deployer          Deployer    // Deploys a Function
	updater           Updater     // Updates a deployed Function
	runner            Runner      // Runs the function locally
	remover           Remover     // Removes remote services
	lister            Lister      // Lists remote services
	describer         Describer
	dnsProvider       DNSProvider      // Provider of DNS services
	domainSearchLimit int              // max dirs to recurse up when deriving domain
	progressListener  ProgressListener // progress listener
}

// Initializer creates the initial/stub Function code on first create.
type Initializer interface {
	// Initialize a Function of the given name, template configuration `
	// (expected signature) using a context template.
	Initialize(runtime, template, path string) error
}

// Builder of function source to runnable image.
type Builder interface {
	// Build a function project with source located at path.
	// returns the image name built.
	Build(path string) (image string, err error)
}

// Pusher of function image to a registry.
type Pusher interface {
	// Push the image of the service function.
	Push(tag string) error
}

// Deployer of function source to running status.
type Deployer interface {
	// Deploy a service function of given name, using given backing image.
	Deploy(name, image string) (address string, err error)
}

// Updater of a deployed service function with new image.
type Updater interface {
	// Deploy a service function of given name, using given backing image.
	Update(name, image string) error
}

// Runner runs the function locally.
type Runner interface {
	// Run the function locally.
	Run(path string) error
}

// Remover of deployed services.
type Remover interface {
	// Remove the service function from remote.
	Remove(name string) error
}

// Lister of deployed services.
type Lister interface {
	// List the service functions currently deployed.
	List() ([]string, error)
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

type Subscription struct {
	Source string `json:"source" yaml:"source"`
	Type   string `json:"type" yaml:"type"`
	Broker string `json:"broker" yaml:"broker"`
}

type FunctionDescription struct {
	Name          string         `json:"name" yaml:"name"`
	Routes        []string       `json:"routes" yaml:"routes"`
	Subscriptions []Subscription `json:"subscriptions" yaml:"subscriptions"`
}

type Describer interface {
	Describe(name string) (description FunctionDescription, err error)
}

// DNSProvider exposes DNS services necessary for serving the Function.
type DNSProvider interface {
	// Provide the given name by routing requests to address.
	Provide(name, address string) (n string)
}

// New client for Function management.
func New(options ...Option) (c *Client, err error) {
	// Instantiate client with static defaults.
	c = &Client{
		initializer:       &noopInitializer{output: os.Stdout},
		builder:           &noopBuilder{output: os.Stdout},
		pusher:            &noopPusher{output: os.Stdout},
		deployer:          &noopDeployer{output: os.Stdout},
		updater:           &noopUpdater{output: os.Stdout},
		runner:            &noopRunner{output: os.Stdout},
		remover:           &noopRemover{output: os.Stdout},
		lister:            &noopLister{output: os.Stdout},
		dnsProvider:       &noopDNSProvider{output: os.Stdout},
		progressListener:  &noopProgressListener{},
		domainSearchLimit: -1, // no recursion limit deriving domain by default.
	}

	// Apply passed options, which take ultimate precidence.
	for _, o := range options {
		o(c)
	}
	return
}

// Option defines a function which when passed to the Client constructor optionally
// mutates private members at time of instantiation.
type Option func(*Client)

// WithVerbose toggles verbose logging.
func WithVerbose(v bool) Option {
	return func(c *Client) {
		c.verbose = v
	}
}

// WithLocal sets the local mode
func WithLocal(l bool) Option {
	return func(c *Client) {
		c.local = l
	}
}

// WithInternal sets the internal (no public route) mode for deployed function.
func WithInternal(i bool) Option {
	return func(c *Client) {
		c.internal = i
	}
}

// WithInitializer provides the concrete implementation of the Function
// initializer (generates stub code on initial create).
func WithInitializer(i Initializer) Option {
	return func(c *Client) {
		c.initializer = i
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

// WithUpdater provides the concrete implementation of an updater.
func WithUpdater(u Updater) Option {
	return func(c *Client) {
		c.updater = u
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

// WithDescriber provides a concrete implementation of a function describer.
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

// WithDomainSearchLimit sets the maximum levels of upward recursion used when
// attempting to derive effective DNS name from root path.  Ignored if DNS was
// explicitly set via WithName.
func WithDomainSearchLimit(limit int) Option {
	return func(c *Client) {
		c.domainSearchLimit = limit
	}
}

// Initialize creates a new function project locally
func (c *Client) Initialize(runtime, template, name, tag, root string) (f *Function, err error) {
	// Create an instance of a function representation at the given root.
	f, err = NewFunction(root)
	if err != nil {
		return nil, err
	}

	// Initialize, writing out a template implementation and a config file.
	// TODO: the function's Initialize parameters are slightly different than
	// the Initializer interface, and can thus cause confusion (one passes an
	// optional name the other passes root path).  This could easily cause
	// confusion and thus we may want to rename Initalizer to the more specific
	// task it performs: ContextTemplateWriter or similar.
	err = f.Initialize(runtime, template, name, tag, c.domainSearchLimit, c.initializer)
	if err != nil {
		return nil, err
	}

	// TODO: Create a status structure and return it for clients to use
	// for output, such as from the CLI.
	fmt.Printf("Created function project %v in %v\n", f.Name, root)
	return f, nil
}

func (c *Client) Build(path string) (image string, err error) {
	return c.builder.Build(path)
}

func (c *Client) Deploy(name, tag string) (address string, err error) {
	err = c.pusher.Push(tag) // First push the image to an image registry
	if err != nil {
		return
	}
	address, err = c.deployer.Deploy(name, tag)
	return address, err
}

func (c *Client) Route(name, address string) (route string) {
	// Ensure that the allocated final address is enabled with the
	// configured DNS provider.
	// NOTE:
	// DNS and TLS are provisioned by Knative Serving + cert-manager,
	// but DNS subdomain CNAME to the Kourier Load Balancer is
	// still manual, and the initial cluster config to suppot the TLD
	// is still manual.
	return c.dnsProvider.Provide(name, address)
}

// Create a service function of the given runtime.
// Name and Root are optional:
// Name is derived from root if possible.
// Root is defaulted to the current working directory.
func (c *Client) Create(runtime, template, name, tag, root string) (err error) {
	c.progressListener.SetTotal(4)
	defer c.progressListener.Done()

	// Initialize, writing out a template implementation and a config file.
	// TODO: the function's Initialize parameters are slightly different than
	// the Initializer interface, and can thus cause confusion (one passes an
	// optional name the other passes root path).  This could easily cause
	// confusion and thus we may want to rename Initalizer to the more specific
	// task it performs: ContextTemplateWriter or similar.
	c.progressListener.Increment("Initializing new function project")
	f, err := c.Initialize(runtime, template, name, tag, root)
	if f == nil || !f.Initialized() {
		return fmt.Errorf("Unable to initialize function")
	}
	if err != nil {
		return err
	}

	// Build the now-initialized service function
	c.progressListener.Increment("Building container image")
	_, err = c.Build(f.Root)
	if err != nil {
		return
	}

	if c.local {
		c.progressListener.Complete("Created function project (local only)")
		return
	}

	// TODO: cluster-local deploy mode
	// if c.internal {
	// 	return errors.New("Deploying in cluster-internal mode (no public route) not yet available.")
	// }

	// Deploy the initialized service function, returning its publicly
	// addressible name for possible registration.
	c.progressListener.Increment("Deploying function to cluster")
	address, err := c.Deploy(f.Name, f.Tag)
	if err != nil {
		return
	}

	// Create an external route to the function
	c.progressListener.Increment("Creating route to function")
	c.Route(f.Name, address)

	c.progressListener.Complete("Create complete")

	// TODO: Create a status structure and return it for clients to use
	// for output, such as from the CLI.
	fmt.Printf("https://%v/\n", address)

	return
}

// Update a previously created service function.
func (c *Client) Update(root string) (err error) {

	// Create an instance of a function representation at the given root.
	f, err := NewFunction(root)
	if err != nil {
		return
	}

	if !f.Initialized() {
		// TODO: this needs a test.
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.  Please create one at this path before updating.", root)
	}

	// Build an image from the current state of the service function's implementation.
	image, err := c.builder.Build(f.Root)
	if err != nil {
		return
	}

	// Push the image for the named service to the configured registry
	if err = c.pusher.Push(image); err != nil {
		return
	}

	// Update the previously-deployed service function, returning its publicly
	// addressible name for possible registration.
	return c.updater.Update(f.Name, image)
}

// Run the function whose code resides at root.
func (c *Client) Run(root string) error {

	// Create an instance of a function representation at the given root.
	f, err := NewFunction(root)
	if err != nil {
		return err
	}

	if !f.Initialized() {
		// TODO: this needs a test.
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.  Please create one at this path in order to run.", root)
	}

	// delegate to concrete implementation of runner entirely.
	return c.runner.Run(f.Root)
}

// List currently deployed service functions.
func (c *Client) List() ([]string, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List()
}

// Describe a function.  Name takes precidence.  If no name is provided,
// the function defined at root is used.
func (c *Client) Describe(name, root string) (fd FunctionDescription, err error) {
	// If name is provided, it takes precidence.
	// Otherwise load the function defined at root.
	if name != "" {
		return c.describer.Describe(name)
	}

	f, err := NewFunction(root)
	if err != nil {
		return fd, err
	}
	if !f.Initialized() {
		return fd, fmt.Errorf("%v is not initialized", f.Name)
	}
	return c.describer.Describe(f.Name)
}

// Remove a function.  Name takes precidence.  If no name is provided,
// the function defined at root is used.
func (c *Client) Remove(name, root string) error {
	// If name is provided, it takes precidence.
	// Otherwise load the function deined at root.
	if name != "" {
		return c.remover.Remove(name)
	}

	f, err := NewFunction(root)
	if err != nil {
		return err
	}
	if !f.Initialized() {
		return fmt.Errorf("%v is not initialized", f.Name)
	}
	return c.remover.Remove(f.Name)
}

// Manual implementations (noops) of required interfaces.
// In practice, the user of this client package (for example the CLI) will
// provide a concrete implementation for all of the interfaces.  For testing or
// development, however, it is usefule that they are defaulted to noops and
// provded only when necessary.  Unit tests for the concrete implementations
// serve to keep the core logic here separate from the imperitive.
// -----------------------------------------------------

type noopInitializer struct{ output io.Writer }

func (n *noopInitializer) Initialize(runtime, template, root string) error {
	fmt.Fprintln(n.output, "skipping initialize: client not initialized WithInitializer")
	return nil
}

type noopBuilder struct{ output io.Writer }

func (n *noopBuilder) Build(path string) (image string, err error) {
	fmt.Fprintln(n.output, "skipping build: client not initialized WithBuilder")
	return "", nil
}

type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(tag string) error {
	fmt.Fprintln(n.output, "skipping push: client not initialized WithPusher")
	return nil
}

type noopDeployer struct{ output io.Writer }

func (n *noopDeployer) Deploy(name, image string) (string, error) {
	fmt.Fprintln(n.output, "skipping deploy: client not initialized WithDeployer")
	return "", nil
}

type noopUpdater struct{ output io.Writer }

func (n *noopUpdater) Update(name, image string) error {
	fmt.Fprintln(n.output, "skipping deploy: client not initialized WithDeployer")
	return nil
}

type noopRunner struct{ output io.Writer }

func (n *noopRunner) Run(root string) error {
	fmt.Fprintln(n.output, "skipping run: client not initialized WithRunner")
	return nil
}

type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(name string) error {
	fmt.Fprintln(n.output, "skipping remove: client not initialized WithRemover")
	return nil
}

type noopLister struct{ output io.Writer }

func (n *noopLister) List() ([]string, error) {
	fmt.Fprintln(n.output, "skipping list: client not initialized WithLister")
	return []string{}, nil
}

type noopDNSProvider struct{ output io.Writer }

func (n *noopDNSProvider) Provide(name, address string) string {
	// Note: at this time manual DNS provisioning required for name -> knative serving netowrk load-balancer
	return ""
}

type noopProgressListener struct{}

func (p *noopProgressListener) SetTotal(i int)     {}
func (p *noopProgressListener) Increment(m string) {}
func (p *noopProgressListener) Complete(m string)  {}
func (p *noopProgressListener) Done()              {}
