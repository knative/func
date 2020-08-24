package faas

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	DefaultNamespace    = "faas"
	DefaultRegistry     = "docker.io"
	DefaultRuntime      = "go"
	DefaultTrigger      = "http"
	DefaultMaxRecursion = 5 // when determining a name from path
)

// Client for managing Function instances.
type Client struct {
	verbose           bool     // print verbose logs
	local             bool     // Run in local-only mode
	internal          bool     // Deploy without publicly accessible route
	builder           Builder  // Builds a runnable image from Function source
	pusher            Pusher   // Pushes the image assocaited with a Function.
	deployer          Deployer // Deploys a Function
	updater           Updater  // Updates a deployed Function
	runner            Runner   // Runs the Function locally
	remover           Remover  // Removes remote services
	lister            Lister   // Lists remote services
	describer         Describer
	dnsProvider       DNSProvider      // Provider of DNS services
	templates         string           // path to extensible templates
	repository        string           // default repo for OCI image tags
	domainSearchLimit int              // max recursion when deriving domain
	progressListener  ProgressListener // progress listener
}

// Builder of Function source to runnable image.
type Builder interface {
	// Build a Function project with source located at path.
	Build(Function) error
}

// Pusher of Function image to a registry.
type Pusher interface {
	// Push the image of the Function.
	Push(Function) error
}

// Deployer of Function source to running status.
type Deployer interface {
	// Deploy a Function of given name, using given backing image.
	Deploy(Function) error
}

// Updater of a deployed Function with new image.
type Updater interface {
	// Update a Function
	Update(Function) error
}

// Runner runs the Function locally.
type Runner interface {
	// Run the Function locally.
	Run(Function) error
}

// Remover of deployed services.
type Remover interface {
	// Remove the Function from remote.
	Remove(name string) error
}

// Lister of deployed services.
type Lister interface {
	// List the Functions currently deployed.
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

// Describer of Functions' remote deployed aspect.
type Describer interface {
	// Describe the running state of the service as reported by the underlyng platform.
	Describe(name string) (description FunctionDescription, err error)
}

type FunctionDescription struct {
	Name          string         `json:"name" yaml:"name"`
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

// New client for Function management.
func New(options ...Option) *Client {
	// Instantiate client with static defaults.
	c := &Client{
		builder:           &noopBuilder{output: os.Stdout},
		pusher:            &noopPusher{output: os.Stdout},
		deployer:          &noopDeployer{output: os.Stdout},
		updater:           &noopUpdater{output: os.Stdout},
		runner:            &noopRunner{output: os.Stdout},
		remover:           &noopRemover{output: os.Stdout},
		lister:            &noopLister{output: os.Stdout},
		dnsProvider:       &noopDNSProvider{output: os.Stdout},
		progressListener:  &noopProgressListener{},
		domainSearchLimit: DefaultMaxRecursion, // no recursion limit deriving domain by default.
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

// WithLocal sets the local mode
func WithLocal(l bool) Option {
	return func(c *Client) {
		c.local = l
	}
}

// WithInternal sets the internal (no public route) mode for deployed Function.
func WithInternal(i bool) Option {
	return func(c *Client) {
		c.internal = i
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

// WithDomainSearchLimit sets the maximum levels of upward recursion used when
// attempting to derive effective DNS name from root path.  Ignored if DNS was
// explicitly set via WithName.
func WithDomainSearchLimit(limit int) Option {
	return func(c *Client) {
		c.domainSearchLimit = limit
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

// WithRepository sets the default registry which is consulted when an image name/tag
// is not explocitly provided.  Can be fully qualified, including the registry
// (ex: 'quay.io/myname') or simply the namespace 'myname' which indicates the
// the use of the default registry.
func WithRepository(repository string) Option {
	return func(c *Client) {
		c.repository = repository
	}
}

// Initialize creates a new Function project locally using the settings
// provided on a Function object.
func (c *Client) Initialize(cfg Function) (err error) {
	// Create Function of the given root path.
	f, err := NewFunction(cfg.Root)
	if err != nil {
		return
	}

	// Do not initialize if already initialized.
	if f.Initialized() {
		err = fmt.Errorf("Function at '%v' already initialized.", cfg.Root)
		return
	}

	// Do not re-initialize unless the directory is empty This is to protect the
	// user from inadvertently overwriting local files which share the same name
	// as those in the applied template if, for instance, an incorrect path is
	// supplied for the new Function. This assertion is that the target path is
	// empty of all but unrelated hidden files.
	if err = assertEmptyRoot(f.Root); err != nil {
		return
	}

	// Set the name to that provided, defaulting to path derivation if empty.
	f.Name = cfg.Name
	if cfg.Name == "" {
		f.Name = pathToDomain(f.Root, c.domainSearchLimit)
		if f.Name == "" {
			err = errors.New("Function name must be deriveable from path or explicitly provided")
			return
		}
	}

	// Assert runtime was provided, or default.
	f.Runtime = cfg.Runtime
	if f.Runtime == "" {
		f.Runtime = DefaultRuntime
	} else {
		f.Runtime = cfg.Runtime
	}

	// Assert trigger was provided, or default.
	f.Trigger = cfg.Trigger
	if f.Trigger == "" {
		f.Trigger = DefaultTrigger
	}

	// Write out a template.
	w := templateWriter{templates: c.templates, verbose: c.verbose}
	if err = w.Write(f.Runtime, f.Trigger, f.Root); err != nil {
		return
	}

	// Write out the config.
	if err = writeConfig(f); err != nil {
		return
	}

	// TODO: Create a status structure and return it for clients to use
	// for output, such as from the CLI.
	if c.verbose {
		fmt.Printf("OK %v %v\n", f.Name, f.Root)
	}
	return
}

// Build the Function at path.  Errors if the funciton is either unloadable or does
// not contain a populated Image.
func (c *Client) Build(path string) (err error) {
	f, err := NewFunction(path)
	if err != nil {
		return
	}

	// Derive Image from the path (preceidence is given to extant config)
	if f.Image, err = DerivedImage(path, c.repository); err != nil {
		return
	}

	if err = c.builder.Build(f); err != nil {
		return
	}

	// Write out config, which will now contain a populated image tag
	// if it had not already
	if err = writeConfig(f); err != nil {
		return
	}

	// TODO: create a statu structure and return it here for optional
	// use by the cli for user echo (rather than rely on verbose mode here)
	if c.verbose {
		fmt.Printf("OK %v\n", f.Image)
	}
	return
}

// Deploy the Function at path.  Errors if the Function has not been
// initialized with an image tag.
func (c *Client) Deploy(path string) (err error) {
	f, err := NewFunction(path)
	if err != nil {
		return
	}
	if f.Image == "" {
		return errors.New("Function needs to have Image tag calculated prior to building.")
	}

	err = c.pusher.Push(f) // First push the image to an image registry
	if err != nil {
		return
	}
	if err = c.deployer.Deploy(f); err != nil {
		return
	}
	if c.verbose {
		// TODO: aspirational.  Should be an actual route echo.
		fmt.Printf("OK https://%v/\n", f.Image)
	}
	return
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

// Create a Function of the given runtime.
// Name and Root are optional:
// Name is derived from root if possible.
// Root is defaulted to the current working directory.
func (c *Client) Create(cfg Function) (err error) {
	c.progressListener.SetTotal(4)
	defer c.progressListener.Done()

	// Initialize, writing out a template implementation and a config file.
	// TODO: the Function's Initialize parameters are slightly different than
	// the Initializer interface, and can thus cause confusion (one passes an
	// optional name the other passes root path).  This could easily cause
	// confusion and thus we may want to rename Initalizer to the more specific
	// task it performs: ContextTemplateWriter or similar.
	c.progressListener.Increment("Initializing new Function project")
	err = c.Initialize(cfg)
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
	err = c.Build(f.Root)
	if err != nil {
		return
	}

	// Deploy the initialized Function, returning its publicly
	// addressible name for possible registration.
	c.progressListener.Increment("Deploying Function to cluster")
	if err = c.Deploy(f.Root); err != nil {
		return
	}

	// Create an external route to the Function
	c.progressListener.Increment("Creating route to Function")
	if err = c.Route(f.Root); err != nil {
		return
	}

	c.progressListener.Complete("Create complete")

	// TODO: use the knative client during deployment such that the actual final
	// route can be returned from the deployment step, passed to the DNS Router
	// for routing actual traffic, and returned here.
	fmt.Printf("https://%v/\n", f.Name)
	return
}

// Update a previously created Function.
func (c *Client) Update(root string) (err error) {

	// Create an instance of a Function representation at the given root.
	f, err := NewFunction(root)
	if err != nil {
		return
	}

	if !f.Initialized() {
		// TODO: this needs a test.
		return fmt.Errorf("the given path '%v' does not contain an initialized Function.  Please create one at this path before updating.", root)
	}

	// Build an image from the current state of the Function's implementation.
	err = c.Build(f.Root)
	if err != nil {
		return
	}

	// reload the Function as it will now have the Image populated if it had not yet been set.
	f, err = NewFunction(f.Root)
	if err != nil {
		return
	}

	// Push the image for the named service to the configured registry
	if err = c.pusher.Push(f); err != nil {
		return
	}

	// Update the previously-deployed Function, returning its publicly
	// addressible name for possible registration.
	return c.updater.Update(f)
}

// Run the Function whose code resides at root.
func (c *Client) Run(root string) error {

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
	return c.runner.Run(f)
}

// List currently deployed Functions.
func (c *Client) List() ([]string, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List()
}

// Describe a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used.
func (c *Client) Describe(name, root string) (fd FunctionDescription, err error) {
	// If name is provided, it takes precidence.
	// Otherwise load the Function defined at root.
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

// Remove a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used.
func (c *Client) Remove(name, root string) error {
	// If name is provided, it takes precidence.
	// Otherwise load the Function deined at root.
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

type noopBuilder struct{ output io.Writer }

func (n *noopBuilder) Build(_ Function) error {
	fmt.Fprintln(n.output, "skipping build: client not initialized WithBuilder")
	return nil
}

type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(_ Function) error {
	fmt.Fprintln(n.output, "skipping push: client not initialized WithPusher")
	return nil
}

type noopDeployer struct{ output io.Writer }

func (n *noopDeployer) Deploy(_ Function) error {
	fmt.Fprintln(n.output, "skipping deploy: client not initialized WithDeployer")
	return nil
}

type noopUpdater struct{ output io.Writer }

func (n *noopUpdater) Update(_ Function) error {
	fmt.Fprintln(n.output, "skipping deploy: client not initialized WithDeployer")
	return nil
}

type noopRunner struct{ output io.Writer }

func (n *noopRunner) Run(_ Function) error {
	fmt.Fprintln(n.output, "skipping run: client not initialized WithRunner")
	return nil
}

type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(string) error {
	fmt.Fprintln(n.output, "skipping remove: client not initialized WithRemover")
	return nil
}

type noopLister struct{ output io.Writer }

func (n *noopLister) List() ([]string, error) {
	fmt.Fprintln(n.output, "skipping list: client not initialized WithLister")
	return []string{}, nil
}

type noopDNSProvider struct{ output io.Writer }

func (n *noopDNSProvider) Provide(_ Function) error {
	// Note: at this time manual DNS provisioning required for name -> knative serving netowrk load-balancer
	return nil
}

type noopProgressListener struct{}

func (p *noopProgressListener) SetTotal(i int)     {}
func (p *noopProgressListener) Increment(m string) {}
func (p *noopProgressListener) Complete(m string)  {}
func (p *noopProgressListener) Done()              {}
