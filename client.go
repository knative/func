package function

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	// DefaultRegistry through which containers of Functions will be shuttled.
	DefaultRegistry = "docker.io"

	// DefaultRuntime is the language runtime for a new Function, including
	// the template written and builder invoked on deploy.
	DefaultRuntime = "node"

	// DefautlTemplate is the default Function signature / environmental context
	// of the resultant function.  All runtimes are expected to have at least
	// one implementation of each supported function signature.  Currently that
	// includes an HTTP Handler ("http") and Cloud Events handler ("events")
	DefaultTemplate = "http"

	// DefaultRepository is the name of the default (builtin) template repository,
	// and is assumed when no template prefix is provided.
	DefaultRepository = "default"
)

// Client for managing Function instances.
type Client struct {
	repositories     *Repositories // Repositories management
	templates        *Templates    // Templates management
	verbose          bool          // print verbose logs
	builder          Builder       // Builds a runnable image from Function source
	pusher           Pusher        // Pushes the image associated with a Function.
	deployer         Deployer      // Deploys or Updates a Function
	runner           Runner        // Runs the Function locally
	remover          Remover       // Removes remote services
	lister           Lister        // Lists remote services
	describer        Describer
	dnsProvider      DNSProvider      // Provider of DNS services
	repository       string           // URL to Git repo (overrides on-disk and embedded)
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

// Status of the Function
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

	// Stopping indicates the process is in the state of stopping, such as when a context cancelation
	// has been received
	Stopping()

	// Done signals a cessation of progress updates.  Should be called in a defer statement to ensure
	// the progress listener can stop any outstanding tasks such as synchronous user updates.
	Done()
}

// Describer of Functions' remote deployed aspect.
type Describer interface {
	// Describe the running state of the service as reported by the underlyng platform.
	Describe(ctx context.Context, name string) (description Info, err error)
}

type Info struct {
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

type Manifest struct {
	Name            string            `yaml:"name"`
	Buildpacks      []string          `yaml:"buildpacks"`
	HealthEndpoints map[string]string `yaml:"healthEndpoints"`
	Builders        map[string]string `yaml:"builders"`
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
		repositories:     &Repositories{},
		templates:        &Templates{},
		builder:          &noopBuilder{output: os.Stdout},
		pusher:           &noopPusher{output: os.Stdout},
		deployer:         &noopDeployer{output: os.Stdout},
		runner:           &noopRunner{output: os.Stdout},
		remover:          &noopRemover{output: os.Stdout},
		lister:           &noopLister{output: os.Stdout},
		dnsProvider:      &noopDNSProvider{output: os.Stdout},
		progressListener: &NoopProgressListener{},
		emitter:          &noopEmitter{},
	}

	// TODO: Repositories default location ($XDG_CONFIG_HOME/func/repositories)
	// will be relocated from CLI to here.
	// c.Repositories.Path = ...

	// Templates management requires the repositories management api
	c.templates.Repositories = c.repositories

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

// WithRepositories sets the location to use for extensible template repositories.
// Extensible template repositories are additional templates that exist on disk and are
// not built into the binary.
func WithRepositories(repositories string) Option {
	return func(c *Client) {
		c.repositories.Path = repositories
	}
}

// WithRepository sets a specific URL to a Git repository from which to pull templates.
// This setting's existence precldes the use of either the inbuilt templates or any
// repositories from the extensible repositories path.
func WithRepository(repository string) Option {
	return func(c *Client) {
		c.repository = repository
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

// Repositories accessor
func (c *Client) Repositories() *Repositories {
	return c.repositories
}

// Templates accessor
func (c *Client) Templates() *Templates {
	return c.templates
}

// New Function.
// Use Create, Build and Deploy independently for lower level control.
func (c *Client) New(ctx context.Context, cfg Function) (err error) {
	c.progressListener.SetTotal(3)
	// Always start a concurrent routine listening for context cancellation.
	// On this event, immediately indicate the task is canceling.
	// (this is useful, for example, when a progress listener is mutating
	// stdout, and a context cancelation needs to free up stdout entirely for
	// the status or error from said cancelltion.
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

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

	// Root must not already be a Function
	//
	// Instantiate a Function struct about the given root path, but
	// immediately exit with error (prior to actual creation) if a
	// Function already existed at that path (Create should never
	// clobber a pre-existing Function)
	f, err := NewFunction(cfg.Root)
	if err != nil {
		return
	}
	if f.Initialized() {
		err = fmt.Errorf("Function at '%v' already initialized.", cfg.Root)
		return
	}

	// Root must not contain any visible files
	//
	// We know from above that the target directory does not contain a Function,
	// but also immediately exit if the target directoy contains any visible files
	// at all, or any of the known hidden files that will be written.
	// This is to ensure that if a user inadvertently chooses an incorrect directory
	// for their new Function, the template and config file writing steps do not
	// cause data loss.
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

	// Assert template was provided, or default.
	f.Template = cfg.Template
	if f.Template == "" {
		f.Template = DefaultTemplate
	}

	// Write out a template.
	w := templateWriter{repositories: c.repositories.Path, url: c.repository, verbose: c.verbose}
	if err = w.Write(f.Runtime, f.Template, f.Root); err != nil {
		return
	}

	// Check if template specifies a builder image. If so, add to configuration
	manifestFilePath := filepath.Join(f.Root, "manifest.yaml")
	if manifestYaml, err := ioutil.ReadFile(manifestFilePath); err == nil {
		// A manifest.yaml file was found. Read the default builder and set in the config file
		manifest := Manifest{}
		if err := yaml.Unmarshal(manifestYaml, &manifest); err == nil {
			f.Builder = manifest.Builders["default"]
			f.Builders = manifest.Builders
			f.Buildpacks = manifest.Buildpacks
			f.HealthEndpoints = manifest.HealthEndpoints
		}
		// Remove the manifest.yaml file so the user is not confused by a
		// configuration file that is only used for project creation/initialization
		if err := os.Remove(manifestFilePath); err != nil {
			if c.verbose {
				fmt.Printf("Cannot remove %v. %v\n", manifestFilePath, err)
			}
		}
	}

	// Now that defaults are set from manifest.yaml for builders/buildpacks
	// be sure to allow configuration to override these

	// If buildpacks are provided, use them
	if len(cfg.Buildpacks) > 0 {
		f.Buildpacks = cfg.Buildpacks
	}

	// If builders are provided use them
	if len(cfg.Builders) > 0 {
		f.Builders = cfg.Builders
		if f.Builders["default"] != "" {
			f.Builder = f.Builders["default"]
		}
	}

	// If a default builder is provided use it
	if cfg.Builder != "" {
		f.Builder = cfg.Builder
	}

	if c.verbose {
		fmt.Printf("Builder:       %s\n", f.Builder)
		if len(f.Buildpacks) > 0 {
			fmt.Println("Buildpacks:")
			for _, b := range f.Buildpacks {
				fmt.Printf("           ... %s\n", b)
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

	m := []string{
		"Still building",
		"Don't give up",
		"This is taking a while",
		"Still building"}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			<-ticker.C
			if len(m) == 0 {
				break
			}
			c.progressListener.Increment(m[0])
			m = m[1:] // remove 0th element
		}
	}()

	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

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
	if runtime.GOOS == "windows" {
		message = fmt.Sprintf("Function image built: %v", f.Image)
	}
	c.progressListener.Increment(message)
	return
}

// Deploy the Function at path.  Errors if the Function has not been
// initialized with an image tag.
func (c *Client) Deploy(ctx context.Context, path string) (err error) {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

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
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

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

// Info for a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used.
func (c *Client) Info(ctx context.Context, name, root string) (d Info, err error) {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()
	// If name is provided, it takes precidence.
	// Otherwise load the Function defined at root.
	if name != "" {
		return c.describer.Describe(ctx, name)
	}

	f, err := NewFunction(root)
	if err != nil {
		return d, err
	}
	if !f.Initialized() {
		return d, fmt.Errorf("%v is not initialized", f.Name)
	}
	return c.describer.Describe(ctx, f.Name)
}

// Remove a Function.  Name takes precidence.  If no name is provided,
// the Function defined at root is used if it exists.
func (c *Client) Remove(ctx context.Context, cfg Function) error {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()
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
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()
	return c.emitter.Emit(ctx, endpoint)
}

// Runtimes available in totality.
// Not all repository/template combinations necessarily exist,
// and further validation is performed when a template+runtime is chosen.
// from a given repository.  This is the global list of all available.
// Returned list is unique and sorted.
func (c *Client) Runtimes() ([]string, error) {
	runtimes := newSortedSet()

	// Gather all runtimes from all repositories
	// into a uniqueness map
	repositories, err := c.Repositories().All()
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repositories {
		for _, runtime := range repo.Runtimes {
			runtimes.Add(runtime)
		}
	}

	// Return a unique, sorted list of runtimes
	return runtimes.Items(), nil
}

// sorted set of strings.
//
// write-optimized and suitable only for fairly small values of N.
// Should this increase dramatically in size, a different implementation,
// such as a linked list, might be more appropriate.
type sortedSet struct {
	members map[string]bool
	sync.Mutex
}

func newSortedSet() *sortedSet {
	return &sortedSet{
		members: make(map[string]bool),
	}
}

func (s *sortedSet) Add(value string) {
	s.Lock()
	s.members[value] = true
	s.Unlock()
}

func (s *sortedSet) Remove(value string) {
	s.Lock()
	delete(s.members, value)
	s.Unlock()
}

func (s *sortedSet) Items() []string {
	s.Lock()
	defer s.Unlock()
	n := []string{}
	for k := range s.members {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Manual implementations (noops) of required interfaces.
// In practice, the user of this client package (for example the CLI) will
// provide a concrete implementation for only the interfaces necessary to
// complete the given command.  Integrators importing the package would
// provide a concrete implementation for all interfaces to be used. To
// enable partial definition (in particular used for testing) they
// are defaulted to noop implementations such that they can be provded
// only when necessary.  Unit tests for the concrete implementations
// serve to keep the core logic here separate from the imperitive, and
// with a minimum of external dependencies.
// -----------------------------------------------------

// Builder
type noopBuilder struct{ output io.Writer }

func (n *noopBuilder) Build(ctx context.Context, _ Function) error { return nil }

// Pusher
type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(ctx context.Context, f Function) (string, error) { return "", nil }

// Deployer
type noopDeployer struct{ output io.Writer }

func (n *noopDeployer) Deploy(ctx context.Context, _ Function) (DeploymentResult, error) {
	return DeploymentResult{}, nil
}

// Runner
type noopRunner struct{ output io.Writer }

func (n *noopRunner) Run(_ context.Context, _ Function) error { return nil }

// Remover
type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(context.Context, string) error { return nil }

// Lister
type noopLister struct{ output io.Writer }

func (n *noopLister) List(context.Context) ([]ListItem, error) { return []ListItem{}, nil }

// Emitter
type noopEmitter struct{}

func (p *noopEmitter) Emit(ctx context.Context, endpoint string) error { return nil }

// DNSProvider
type noopDNSProvider struct{ output io.Writer }

func (n *noopDNSProvider) Provide(_ Function) error { return nil }

// ProgressListener
type NoopProgressListener struct{}

func (p *NoopProgressListener) SetTotal(i int)     {}
func (p *NoopProgressListener) Increment(m string) {}
func (p *NoopProgressListener) Complete(m string)  {}
func (p *NoopProgressListener) Stopping()          {}
func (p *NoopProgressListener) Done()              {}
