package faas

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const DefaultNamespace = "faas"

// Client for a given Service Function.
type Client struct {
	verbose           bool        // print verbose logs
	local             bool        // Run in local-only mode
	internal          bool        // Deploy without publicly accessible route
	initializer       Initializer // Creates initial local function implementation
	builder           Builder     // Builds a runnable image from function source
	pusher            Pusher      // Pushes a built image to a registry
	deployer          Deployer    // Deploys a Service Function
	updater           Updater     // Updates a deployed Service Function
	runner            Runner      // Runs the function locally
	remover           Remover     // Removes remote services
	lister            Lister      // Lists remote services
	describer         Describer
	dnsProvider       DNSProvider // Provider of DNS services
	domainSearchLimit int         // max dirs to recurse up when deriving domain
}

// Initializer creates the initial/stub Service Function code on first create.
type Initializer interface {
	// Initialize a Service Function of the given name, using the templates for
	// the given language, written into the given path.
	Initialize(name, language, path string) error
}

// Builder of function source to runnable image.
type Builder interface {
	// Build a service function of the given name with source located at path.
	// returns the image name built.
	Build(name, path string) (image string, err error)
}

// Pusher of function image to a registry.
type Pusher interface {
	// Push the image of the service function.
	Push(image string) error
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

// DNSProvider exposes DNS services necessary for serving the Service Function.
type DNSProvider interface {
	// Provide the given name by routing requests to address.
	Provide(name, address string)
}

// New client for Service Function management.
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

// WithInitializer provides the concrete implementation of the Service Function
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

func WithDescriber(describer Describer) Option {
	return func(c *Client) {
		c.describer = describer
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

// Create a service function of the given language.
// Name and Root are optional:
// Name is derived from root if possible.
// Root is defaulted to the current working directory.
func (c *Client) Create(language, name, root string) (err error) {
	// Create an instance of a function representation at the given root.
	f, err := NewFunction(root)
	if err != nil {
		return
	}

	// Initialize, writing out a template implementation and a config file.
	err = f.Initialize(language, name, c.domainSearchLimit, c.initializer)
	if err != nil {
		return
	}

	// Build the now-initialized service function
	image, err := c.builder.Build(f.name, f.root)
	if err != nil {
		return
	}

	// If running local-only, we're done.
	if c.local {
		return
	}

	// Push the image for the names service to the configured registry
	if err = c.pusher.Push(image); err != nil {
		return
	}

	// TODO: cluster-local deploy mode
	if c.internal {
		return errors.New("Deploying in cluster-internal mode (no public route) not yet available.")
	}

	// Deploy the initialized service function, returning its publicly
	// addressible name for possible registration.
	address, err := c.deployer.Deploy(f.name, image)
	if err != nil {
		return
	}

	// Ensure that the allocated final address is enabled with the
	// configured DNS provider.
	// NOTE:
	// DNS and TLS are provisioned by Knative Serving + cert-manager,
	// but DNS subdomain CNAME to the Kourier Load Balancer is
	// still manual, and the initial cluster config to suppot the TLD
	// is still manual.
	c.dnsProvider.Provide(f.name, address)

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
		return errors.New(fmt.Sprintf("the given path '%v' does not contain an initialized Service Function.  Please create one at this path before updating.", root))
	}

	// Build an image from the current state of the service function's implementation.
	image, err := c.builder.Build(f.name, f.root)
	if err != nil {
		return
	}

	// Push the image for the named service to the configured registry
	if err = c.pusher.Push(image); err != nil {
		return
	}

	// Update the previously-deployed service function, returning its publicly
	// addressible name for possible registration.
	return c.updater.Update(f.name, image)
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
		return errors.New(fmt.Sprintf("the given path '%v' does not contain an initialized Service Function.  Please create one at this path in order to run.", root))
	}

	// delegate to concrete implementation of runner entirely.
	return c.runner.Run(f.root)
}

// List currently deployed service functions.
func (c *Client) List() ([]string, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List()
}

func (c *Client) Describe(name string) (FunctionDescription, error) {
	// delegate to concrete implementation of describer entirely.
	return c.describer.Describe(name)
}

// Remove a function from remote.  No local files are affected.
func (c *Client) Remove(name string) error {
	// delegate to concrete implementation of remover entirely.
	return c.remover.Remove(name)
}

// Manual implementations (noops) of required interfaces.
// In practice, the user of this client package (for example the CLI) will
// provide a concrete implementation for all of the interfaces.  For testing or
// development, however, it is usefule that they are defaulted to noops and
// provded only when necessary.  Unit tests for the concrete implementations
// serve to keep the core logic here separate from the imperitive.
// -----------------------------------------------------

type noopDNSProvider struct{ output io.Writer }

func (n *noopDNSProvider) Provide(name, address string) {
	fmt.Fprintln(n.output, "skipping DNS update: client not initialized WithDNSProvider")
}

type noopInitializer struct{ output io.Writer }

func (n *noopInitializer) Initialize(name, language, root string) error {
	fmt.Fprintln(n.output, "skipping initialize: client not initialized WithInitializer")
	return nil
}

type noopBuilder struct{ output io.Writer }

func (n *noopBuilder) Build(name, root string) (image string, err error) {
	fmt.Fprintln(n.output, "skipping build: client not initialized WithBuilder")
	return "", nil
}

type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(image string) error {
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
