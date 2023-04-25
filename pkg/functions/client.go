package functions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"knative.dev/func/pkg/utils"
)

const (
	// DefaultRegistry through which containers of functions will be shuttled.
	DefaultRegistry = "docker.io"

	// DefaultTemplate is the default function signature / environmental context
	// of the resultant function.  All runtimes are expected to have at least
	// one implementation of each supported function signature.  Currently that
	// includes an HTTP Handler ("http") and Cloud Events handler ("events")
	DefaultTemplate = "http"
)

// Client for managing function instances.
type Client struct {
	repositoriesPath  string            // path to repositories
	repositoriesURI   string            // repo URI (overrides repositories path)
	verbose           bool              // print verbose logs
	builder           Builder           // Builds a runnable image source
	pusher            Pusher            // Pushes function image to a remote
	deployer          Deployer          // Deploys or Updates a function
	runner            Runner            // Runs the function locally
	remover           Remover           // Removes remote services
	lister            Lister            // Lists remote services
	describer         Describer         // Describes function instances
	dnsProvider       DNSProvider       // Provider of DNS services
	registry          string            // default registry for OCI image tags
	progressListener  ProgressListener  // progress listener
	repositories      *Repositories     // Repositories management
	templates         *Templates        // Templates management
	instances         *InstanceRefs     // Function Instances management
	transport         http.RoundTripper // Customizable internal transport
	pipelinesProvider PipelinesProvider // CI/CD pipelines management
}

// ErrNotBuilt indicates the function has not yet been built.
var ErrNotBuilt = errors.New("not built")

// ErrNameRequired indicates the operation requires a name to complete.
var ErrNameRequired = errors.New("name required")

// ErrRegistryRequired indicates the operation requires a registry to complete.
var ErrRegistryRequired = errors.New("registry required to build function, please set with `--registry` or the FUNC_REGISTRY environment variable")

// Builder of function source to runnable image.
type Builder interface {
	// Build a function project with source located at path.
	Build(context.Context, Function) error
}

// Pusher of function image to a registry.
type Pusher interface {
	// Push the image of the function.
	// Returns Image Digest - SHA256 hash of the produced image
	Push(ctx context.Context, f Function) (string, error)
}

// Deployer of function source to running status.
type Deployer interface {
	// Deploy a function of given name, using given backing image.
	Deploy(context.Context, Function) (DeploymentResult, error)
}

type DeploymentResult struct {
	Status    Status
	URL       string
	Namespace string
}

// Status of the function from the DeploymentResult
type Status int

const (
	Failed Status = iota
	Deployed
	Updated
)

// Runner runs the function locally.
type Runner interface {
	// Run the function, returning a Job with metadata, error channels, and
	// a stop function.The process can be stopped by running the returned stop
	// function, either on context cancellation or in a defer.
	Run(context.Context, Function) (*Job, error)
}

// Remover of deployed services.
type Remover interface {
	// Remove the function from remote.
	Remove(ctx context.Context, name string) error
}

// Lister of deployed functions.
type Lister interface {
	// List the functions currently deployed.
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
	// Complete signals completion, which is expected to be somewhat different
	// than a step increment.
	Complete(message string)
	// Stopping indicates the process is in the state of stopping, such as when a
	// context cancelation has been received
	Stopping()
	// Done signals a cessation of progress updates.  Should be called in a defer
	// statement to ensure the progress listener can stop any outstanding tasks
	// such as synchronous user updates.
	Done()
}

// Describer of function instances
type Describer interface {
	// Describe the named function in the remote environment.
	Describe(ctx context.Context, name string) (Instance, error)
}

// Instance data about the runtime state of a function in a given environment.
//
// A function instance is a logical running function space, which share
// a unique route (or set of routes).  Due to autoscaling and load balancing,
// there is a one to many relationship between a given route and processes.
// By default the system creates the 'local' and 'remote' named instances
// when a function is run (locally) and deployed, respectively.
// See the .Instances(f) accessor for the map of named environments to these
// function information structures.
type Instance struct {
	// Route is the primary route of a function instance.
	Route string
	// Routes is the primary route plus any other route at which the function
	// can be contacted.
	Routes        []string       `json:"routes" yaml:"routes"`
	Name          string         `json:"name" yaml:"name"`
	Image         string         `json:"image" yaml:"image"`
	Namespace     string         `json:"namespace" yaml:"namespace"`
	Subscriptions []Subscription `json:"subscriptions" yaml:"subscriptions"`
}

// Subscriptions currently active to event sources
type Subscription struct {
	Source string `json:"source" yaml:"source"`
	Type   string `json:"type" yaml:"type"`
	Broker string `json:"broker" yaml:"broker"`
}

// DNSProvider exposes DNS services necessary for serving the function.
type DNSProvider interface {
	// Provide the given name by routing requests to address.
	Provide(Function) error
}

// PipelinesProvider manages lifecyle of CI/CD pipelines used by a function
type PipelinesProvider interface {
	Run(context.Context, Function) error
	Remove(context.Context, Function) error
	ConfigurePAC(context.Context, Function, any) error
	RemovePAC(context.Context, Function, any) error
}

// New client for function management.
func New(options ...Option) *Client {
	// Instantiate client with static defaults.
	c := &Client{
		builder:           &noopBuilder{output: os.Stdout},
		pusher:            &noopPusher{output: os.Stdout},
		deployer:          &noopDeployer{output: os.Stdout},
		runner:            &noopRunner{output: os.Stdout},
		remover:           &noopRemover{output: os.Stdout},
		lister:            &noopLister{output: os.Stdout},
		describer:         &noopDescriber{output: os.Stdout},
		dnsProvider:       &noopDNSProvider{output: os.Stdout},
		progressListener:  &NoopProgressListener{},
		pipelinesProvider: &noopPipelinesProvider{},
		transport:         http.DefaultTransport,
	}
	for _, o := range options {
		o(c)
	}

	// Initialize sub-managers using now-fully-initialized client.
	c.repositories = newRepositories(c)
	c.templates = newTemplates(c)
	c.instances = newInstances(c)

	return c
}

// RepositoriesPath accesses the currently effective repositories path,
// which can be set using the WithRepositoriesPath option.
func (c *Client) RepositoriesPath() (path string) {
	path = c.repositories.Path()
	return
}

// RepositoriesPath is a convenience method for accessing the default path to
// repositories that will be used by new instances of a Client unless options
// such as WithRepositoriesPath are used to override.
// The path will be created if it does not already exist.
func RepositoriesPath() string {
	return New().RepositoriesPath()
}

// OPTIONS
// ---------

// Option defines a function which when passed to the Client constructor
// optionally mutates private members at time of instantiation.
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

// WithRepositoriesPath sets the location on disk to use for extensible template
// repositories.  Extensible template repositories are additional templates
// that exist on disk and are not built into the binary.
func WithRepositoriesPath(path string) Option {
	return func(c *Client) {
		c.repositoriesPath = path
	}
}

// WithRepository sets a specific URL to a Git repository from which to pull
// templates.  This setting's existence precldes the use of either the inbuilt
// templates or any repositories from the extensible repositories path.
func WithRepository(uri string) Option {
	return func(c *Client) {
		c.repositoriesURI = uri
	}
}

// WithRegistry sets the default registry which is consulted when an image
// name is not explicitly provided.  Can be fully qualified, including the
// registry and namespace (ex: 'quay.io/myname') or simply the namespace
// (ex: 'myname').
func WithRegistry(registry string) Option {
	return func(c *Client) {
		c.registry = registry
	}
}

// WithTransport sets a custom transport to use internally.
func WithTransport(t http.RoundTripper) Option {
	return func(c *Client) {
		c.transport = t
	}
}

// WithPipelinesProvider sets implementation of provider responsible for CI/CD pipelines
func WithPipelinesProvider(pp PipelinesProvider) Option {
	return func(c *Client) {
		c.pipelinesProvider = pp
	}
}

// ACCESSORS
// ---------

// Repositories accessor
func (c *Client) Repositories() *Repositories {
	return c.repositories
}

// Templates accessor
func (c *Client) Templates() *Templates {
	return c.templates
}

// Instances accessor
func (c *Client) Instances() *InstanceRefs {
	return c.instances
}

// Repository accessor returns the default registry for use when building
// Functions which do not specify Registry or Image name explicitly.
func (c *Client) Registry() string {
	return c.registry
}

// Runtimes available in totality.
// Not all repository/template combinations necessarily exist,
// and further validation is performed when a template+runtime is chosen.
// from a given repository.  This is the global list of all available.
// Returned list is unique and sorted.
func (c *Client) Runtimes() ([]string, error) {
	runtimes := utils.NewSortedSet()

	// Gather all runtimes from all repositories
	// into a uniqueness map
	repositories, err := c.Repositories().All()
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repositories {
		for _, runtime := range repo.Runtimes {
			runtimes.Add(runtime.Name)
		}
	}

	// Return a unique, sorted list of runtimes
	return runtimes.Items(), nil
}

// LIFECYCLE METHODS
// -----------------

// Apply (aka upsert)
//
// The general-purpose high-level method to initiate a synchronization of
// a function's source code and it's deployed instance(s).
//
// Invokes all lower-level methods, including initialization, as necessary to
// create a running function whose source code and metadata match that provided
// by the passed function instance, returning the final route and any errors.
func (c *Client) Apply(ctx context.Context, f Function) (string, Function, error) {
	if !f.Initialized() {
		return c.New(ctx, f)
	} else {
		return c.Update(ctx, f)
	}
}

// Update function
//
// Updates a function which has already been initialized to run the latest
// source code.
//
// Use Init, Build, Push and Deploy independently for lower level control.
// Returns final primary route to the Function and any errors.
func (c *Client) Update(ctx context.Context, f Function) (string, Function, error) {
	if !f.Initialized() {
		return "", f, ErrNotInitialized
	}
	var err error
	if f, err = c.Build(ctx, f); err != nil {
		return "", f, err
	}
	if f, err = c.Push(ctx, f); err != nil {
		return "", f, err
	}
	if f, err = c.Deploy(ctx, f); err != nil {
		return "", f, err
	}
	return c.Route(ctx, f)
}

// New function.
//
// Creates a new running function from the path indicated by the config
// Function. Used by Apply when the path is not yet an initialized function.
// Errors if the path is alrady an initialized function.
//
// Use Init, Build, Push, Deploy etc. independently for lower level control.
// Returns the primary route to the function or error.
func (c *Client) New(ctx context.Context, cfg Function) (string, Function, error) {
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

	var route string
	// Init the path as a new Function
	f, err := c.Init(cfg)
	if err != nil {
		return route, cfg, err
	}

	// Build the now-initialized function
	c.progressListener.Increment("Building container image")
	if f, err = c.Build(ctx, f); err != nil {
		return route, f, err
	}

	// Push the produced function image
	c.progressListener.Increment("Pushing container image to registry")
	if f, err = c.Push(ctx, f); err != nil {
		return route, f, err
	}

	// Deploy the initialized function, returning its publicly
	// addressible name for possible registration.
	c.progressListener.Increment("Deploying function to cluster")
	if f, err = c.Deploy(ctx, f); err != nil {
		return route, f, err
	}

	// Create an external route to the function
	c.progressListener.Increment("Creating route to function")
	if route, f, err = c.Route(ctx, f); err != nil {
		return route, f, err
	}

	c.progressListener.Complete("Done")

	return route, f, err
}

// Initialize a new function with the given function struct defaults.
// Does not build/push/deploy. Local FS changes only.  For higher-level
// control see New or Apply.
//
// <path> will default to the absolute path of the current working directory.
// <name> will default to the current working directory.
// When <name> is provided but <path> is not, a directory <name> is created
// in the current working directory and used for <path>.
func (c *Client) Init(cfg Function) (Function, error) {
	// convert Root path to absolute
	var err error
	oldRoot := cfg.Root
	cfg.Root, err = filepath.Abs(cfg.Root)
	cfg.SpecVersion = LastSpecVersion()
	if err != nil {
		return cfg, err
	}

	// Create project root directory, if it doesn't already exist
	if err = os.MkdirAll(cfg.Root, 0755); err != nil {
		return cfg, err
	}

	// Create should never clobber a pre-existing function
	hasFunc, err := hasInitializedFunction(cfg.Root)
	if err != nil {
		return cfg, err
	}
	if hasFunc {
		return cfg, fmt.Errorf("function at '%v' already initialized", cfg.Root)
	}

	// Path is defaulted to the current working directory
	if cfg.Root == "" {
		if cfg.Root, err = os.Getwd(); err != nil {
			return cfg, err
		}
	}

	// Name is defaulted to the directory of the given path.
	if cfg.Name == "" {
		cfg.Name = nameFromPath(cfg.Root)
	}

	// The path for the new function should not have any contentious files
	// (hidden files OK, unless it's one used by func)
	if err := assertEmptyRoot(cfg.Root); err != nil {
		return cfg, err
	}

	// Create a new function (in memory)
	f := NewFunctionWith(cfg)

	// Create a .func diretory which is also added to a .gitignore
	if err = f.ensureRuntimeDir(); err != nil {
		return f, err
	}

	// Write out the new function's Template files.
	// Templates contain values which may result in the function being mutated
	// (default builders, etc), so a new (potentially mutated) function is
	// returned from Templates.Write
	err = c.Templates().Write(&f)
	if err != nil {
		return f, err
	}

	// Mark the function as having been created
	f.Created = time.Now()
	err = f.Write()
	if err != nil {
		return f, err
	}
	// Load the now-initialized function.
	return NewFunction(oldRoot)
}

// Build the function at path. Errors if the function is either unloadable or does
// not contain a populated Image.
func (c *Client) Build(ctx context.Context, f Function) (Function, error) {
	c.progressListener.Increment("Building function image")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// If not logging verbosely, the ongoing progress of the build will not
	// be streaming to stdout, and the lack of activity has been seen to cause
	// users to prematurely exit due to the sluggishness of pulling large images
	if !c.verbose {
		c.printBuildActivity(ctx) // print friendly messages until context is canceled
	}

	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been yet defined (not yet built/deployed), calculate.
	// Image name is stored on the function for later use by deploy, etc.
	// TODO: write this to .func/build instead, and populate f.Image on deploy
	// such that local builds do not dirty the work tree.
	var err error
	if f.Image == "" {
		if f.Image, err = f.ImageName(); err != nil {
			return f, err
		}
	}

	if err = c.builder.Build(ctx, f); err != nil {
		return f, err
	}

	f, err = f.updateBuildStamp()
	if err != nil {
		return f, err
	}

	// TODO: create a status structure and return it here for optional
	// use by the cli for user echo (rather than rely on verbose mode here)
	message := fmt.Sprintf("🙌 Function image built: %v", f.Image)
	if runtime.GOOS == "windows" {
		message = fmt.Sprintf("Function image built: %v", f.Image)
	}
	c.progressListener.Increment(message)
	return f, err
}

func (c *Client) printBuildActivity(ctx context.Context) {
	m := []string{
		"Still building",
		"Still building",
		"Yes, still building",
		"Don't give up on me",
		"Still building",
		"This is taking a while",
	}
	i := 0
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.progressListener.Increment(m[i])
				i++
				i = i % len(m)
			case <-ctx.Done():
				c.progressListener.Stopping()
				ticker.Stop()
				return
			}
		}
	}()
}

type DeployParams struct {
	skipBuiltCheck bool
}
type DeployOption func(f *DeployParams)

func WithDeploySkipBuildCheck(skipBuiltCheck bool) DeployOption {
	return func(f *DeployParams) {
		f.skipBuiltCheck = skipBuiltCheck
	}
}

// Deploy the function at path. Errors if the function has not been built.
func (c *Client) Deploy(ctx context.Context, f Function, opts ...DeployOption) (Function, error) {

	deployParams := &DeployParams{skipBuiltCheck: false}
	for _, opt := range opts {
		opt(deployParams)
	}

	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	// Functions must be built (have an associated image) before being deployed.
	// Note that externally built images may be specified in the func.yaml
	if !deployParams.skipBuiltCheck && !f.Built() {
		return f, ErrNotBuilt
	}

	// Functions must have a name to be deployed (a path on the network at which
	// it should take up residence.
	if f.Name == "" {
		return f, ErrNameRequired
	}

	// Deploy a new or Update the previously-deployed function
	c.progressListener.Increment("⬆️  Deploying function to the cluster")
	result, err := c.deployer.Deploy(ctx, f)
	if err != nil {
		fmt.Printf("deploy error: %v\n", err)
		return f, err
	}

	// Update the function with the namespace into which the function was
	// deployed
	f.Deploy.Namespace = result.Namespace

	if result.Status == Deployed {
		c.progressListener.Increment(fmt.Sprintf("✅ Function deployed in namespace %q and exposed at URL: \n   %v", result.Namespace, result.URL))
	} else if result.Status == Updated {
		c.progressListener.Increment(fmt.Sprintf("✅ Function updated in namespace %q and exposed at URL: \n   %v", result.Namespace, result.URL))
	}

	// Metadata generated from deploying (namespace) should not trigger a rebuild
	// through a staleness check, so update the build stamp we checked earlier.
	return f.updateBuildStamp()
}

// RunPipeline runs a Pipeline to build and deploy the function.
// Returned function contains applicable registry and deployed image name.
func (c *Client) RunPipeline(ctx context.Context, f Function) (Function, error) {
	var err error
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been yet defined (not yet built/deployed), calculate.
	// Image name is stored on the function for later use by deploy, etc.
	if f.Image == "" {
		if f.Image, err = f.ImageName(); err != nil {
			return f, err
		}
	}

	// Build and deploy function using Pipeline
	if err := c.pipelinesProvider.Run(ctx, f); err != nil {
		return f, fmt.Errorf("failed to run pipeline: %w", err)
	}

	return f, nil
}

// ConfigurePAC generates Pipeline resources on the local filesystem,
// on the cluster and also on the remote git provider (ie. GitHub, GitLab or BitBucket repo)
func (c *Client) ConfigurePAC(ctx context.Context, f Function, metadata any) error {
	var err error
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been yet defined (not yet built/deployed), calculate.
	// Image name is stored on the function for later use by deploy, etc.
	if f.Image == "" {
		if f.Image, err = f.ImageName(); err != nil {
			return err
		}
	}

	// saves image name/registry to function's metadata (func.yaml)
	if err = f.Write(); err != nil {
		return err
	}

	// Build and deploy function using Pipeline
	if err := c.pipelinesProvider.ConfigurePAC(ctx, f, metadata); err != nil {
		return fmt.Errorf("failed to run generate pipeline: %w", err)
	}

	return nil
}

// RemovePAC deletes generated Pipeline as Code resources on the local filesystem and on the cluster
func (c *Client) RemovePAC(ctx context.Context, f Function, metadata any) error {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	// Build and deploy function using Pipeline
	if err := c.pipelinesProvider.RemovePAC(ctx, f, metadata); err != nil {
		return fmt.Errorf("failed to remove git related resources: %w", err)
	}

	return nil
}

// Route returns the current primary route to the function at root.
//
// Note that local instances of the Function created by the .Run
// method are not considered here.  This method is intended to specifically
// apply to the logical group of function instances actually available as
// network sevices; this excludes local testing instances.
//
// For access to these local test function instances routes, use the instances
// manager directly ( see .Instances().Get() ).
func (c *Client) Route(ctx context.Context, f Function) (string, Function, error) {
	// Ensure that the allocated final address is enabled with the
	// configured DNS provider.
	// NOTE:
	// DNS and TLS are provisioned by Knative Serving + cert-manager,
	// but DNS subdomain CNAME to the Kourier Load Balancer is
	// still manual, and the initial cluster config to suppot the TLD
	// is still manual.
	if err := c.dnsProvider.Provide(f); err != nil {
		return "", f, err
	}

	// Return the correct route.
	instance, err := c.Instances().Remote(ctx, "", f.Root)
	if err != nil {
		return "", f, err
	}
	return instance.Route, f, nil
}

// Run the function whose code resides at root.
// On start, the chosen port is sent to the provided started channel
func (c *Client) Run(ctx context.Context, f Function) (job *Job, err error) {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	if !f.Initialized() {
		// TODO: this needs a test.
		err = fmt.Errorf("the given path '%v' does not contain an initialized "+
			"function.  Please create one at this path in order to run", f.Root)
		return nil, err
	}

	// Run the function, which returns a Job for use interacting (at arms length)
	// with that running task (which is likely inside a container process).
	if job, err = c.runner.Run(ctx, f); err != nil {
		return
	}

	// Return to the caller the effective port, a function to call to trigger
	// stop, and a channel on which can be received runtime errors.
	return job, nil
}

// Describe a function.  Name takes precedence.  If no name is provided,
// the function defined at root is used.
func (c *Client) Describe(ctx context.Context, name string, f Function) (d Instance, err error) {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()
	// If name is provided, it takes precedence.
	// Otherwise load the function defined at root.
	if name != "" {
		return c.describer.Describe(ctx, name)
	}

	if !f.Initialized() {
		return d, fmt.Errorf("function not initialized: %v", f.Root)
	}
	if f.Name == "" {
		return d, fmt.Errorf("unable to describe without a name. %v", ErrNameRequired)
	}
	return c.describer.Describe(ctx, f.Name)
}

// List currently deployed functions.
func (c *Client) List(ctx context.Context) ([]ListItem, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List(ctx)
}

// Remove a function.  Name takes precedence.  If no name is provided,
// the function defined at root is used if it exists.
func (c *Client) Remove(ctx context.Context, cfg Function, deleteAll bool) error {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()
	// If name is provided, it takes precedence.
	// Otherwise load the function defined at root.
	functionName := cfg.Name
	if cfg.Name == "" {
		f, err := NewFunction(cfg.Root)
		if err != nil {
			return err
		}
		if !f.Initialized() {
			return fmt.Errorf("function at %v can not be removed unless initialized. Try removing by name", f.Root)
		}
		functionName = f.Name
		cfg = f
	}
	if functionName == "" {
		return ErrNameRequired
	}

	// Delete Knative Service and dependent resources in parallel
	c.progressListener.Increment(fmt.Sprintf("Removing Knative Service: %v", functionName))
	errChan := make(chan error)
	go func() {
		errChan <- c.remover.Remove(ctx, functionName)
	}()

	var errResources error
	if deleteAll {
		c.progressListener.Increment(fmt.Sprintf("Removing Knative Service '%v' and all dependent resources", functionName))
		errResources = c.pipelinesProvider.Remove(ctx, cfg)
	}

	errService := <-errChan

	if errService != nil && errResources != nil {
		return fmt.Errorf("%s\n%s", errService, errResources)
	} else if errResources != nil {
		return errResources
	}
	return errService
}

// Invoke is a convenience method for triggering the execution of a function
// for testing and development.  Returned is a map of metadata and a stringified
// version of the content.
// The target argument is optional, naming the running instance of the function
// which should be invoked.  This can be the literal names "local" or "remote",
// or can be a URL to an arbitrary endpoint.  If not provided, a running local
// instance is preferred, with the remote function triggered if there is no
// locally running instance.
// Example:
//
//	myClient.Invoke(myContext, myFunction, "local", NewInvokeMessage())
//
// The message sent to the function is defined by the invoke message.
// See NewInvokeMessage for its defaults.
// Functions are invoked in a manner consistent with the settings defined in
// their metadata.  For example HTTP vs CloudEvent
func (c *Client) Invoke(ctx context.Context, root string, target string, m InvokeMessage) (metadata map[string][]string, body string, err error) {
	go func() {
		<-ctx.Done()
		c.progressListener.Stopping()
	}()

	f, err := NewFunction(root)
	if err != nil {
		return
	}
	// See invoke.go for implementation details
	return invoke(ctx, c, f, target, m, c.verbose)
}

// Push the image for the named service to the configured registry
func (c *Client) Push(ctx context.Context, f Function) (Function, error) {
	if !f.Built() {
		return f, ErrNotBuilt
	}
	var err error
	if f.ImageDigest, err = c.pusher.Push(ctx, f); err != nil {
		return f, err
	}

	// Metadata generated from pushing (ImageDigest) should not trigger a rebuild
	// through a staleness check, so update the build stamp we checked earlier.
	return f.updateBuildStamp()
}

// DEFAULTS
// ---------

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

func (n *noopRunner) Run(context.Context, Function) (job *Job, err error) {
	return
}

// Remover
type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(context.Context, string) error { return nil }

// Lister
type noopLister struct{ output io.Writer }

func (n *noopLister) List(context.Context) ([]ListItem, error) { return []ListItem{}, nil }

// Describer
type noopDescriber struct{ output io.Writer }

func (n *noopDescriber) Describe(context.Context, string) (Instance, error) {
	return Instance{}, nil
}

// PipelinesProvider
type noopPipelinesProvider struct{}

func (n *noopPipelinesProvider) Run(ctx context.Context, _ Function) error    { return nil }
func (n *noopPipelinesProvider) Remove(ctx context.Context, _ Function) error { return nil }
func (n *noopPipelinesProvider) ConfigurePAC(ctx context.Context, _ Function, _ any) error {
	return nil
}
func (n *noopPipelinesProvider) RemovePAC(ctx context.Context, _ Function, _ any) error {
	return nil
}

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
