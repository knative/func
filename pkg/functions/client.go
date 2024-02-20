package functions

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"knative.dev/func/pkg/scaffolding"
	"knative.dev/func/pkg/utils"
)

const (
	// DefaultRegistry through which containers of functions will be shuttled.
	DefaultRegistry = "index.docker.io"

	// DefaultTemplate is the default function signature / environmental context
	// of the resultant function.  All runtimes are expected to have at least
	// one implementation of each supported function signature.  Currently that
	// includes an HTTP Handler ("http") and Cloud Events handler ("events")
	DefaultTemplate = "http"

	// DefaultStartTimeout is the suggested startup timeout to use by
	// runner implementations.
	DefaultStartTimeout = 60 * time.Second
)

var (
	// DefaultPlatforms is a suggestion to builder implementations which
	// platforms should be the default.  Due to spotty implementation support
	// use of this set is left up to the discretion of the builders
	// themselves.  In the event the builder receives build options which
	// specify a set of platforms to use in leau of the default (see the
	// BuildWithPlatforms functionl option), the builder should return
	// an error if the request can not proceed.
	DefaultPlatforms = []Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm64"},
		{OS: "linux", Architecture: "arm", Variant: "v7"}, // eg. RPiv4
	}
)

// Platform upon which a function may run
type Platform struct {
	OS           string
	Architecture string
	Variant      string
}

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
	repositories      *Repositories     // Repositories management
	templates         *Templates        // Templates management
	instances         *InstanceRefs     // Function Instances management
	transport         http.RoundTripper // Customizable internal transport
	pipelinesProvider PipelinesProvider // CI/CD pipelines management
	startTimeout      time.Duration     // default start timeout for all runs
}

// Builder of function source to runnable image.
type Builder interface {
	// Build a function project with source located at path.
	Build(context.Context, Function, []Platform) error
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
	// a stop function.  The process can be stopped by running the returned stop
	// function, either on context cancellation or in a defer.
	// The duration is the time to wait for the job to start.
	Run(context.Context, Function, time.Duration) (*Job, error)
}

// Remover of deployed services.
type Remover interface {
	// Remove the function from remote.
	Remove(ctx context.Context, name string, namespace string) error
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
// See the .InstanceRefs(f) accessor for the map of named environments to these
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
	Run(context.Context, Function) (string, string, error)
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
		remover:           &noopRemover{output: os.Stdout},
		lister:            &noopLister{output: os.Stdout},
		describer:         &noopDescriber{output: os.Stdout},
		dnsProvider:       &noopDNSProvider{output: os.Stdout},
		pipelinesProvider: &noopPipelinesProvider{},
		transport:         http.DefaultTransport,
		startTimeout:      DefaultStartTimeout,
	}
	c.runner = newDefaultRunner(c, os.Stdout, os.Stderr)
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

// WithStartTimeout sets a custom default timeout for functions which do not
// define their own.  This is useful in situations where the client is
// operating in a restricted environment and all functions tend to take longer
// to start up than usual, or when the client is running functions which
// in general take longer to start.  If a timeout is specified on the
// function itself, that will take precidence.  Use the RunWithTimeout option
// on the Run method to specify a timeout with precidence.
func WithStartTimeout(t time.Duration) Option {
	return func(c *Client) {
		c.startTimeout = t
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

	// Gather all runtimes from all repositories into a uniqueness map
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
	if f.Initialized() {
		return c.Update(ctx, f)
	} else {
		return c.New(ctx, f)
	}
}

// Update function
//
// Updates a function which has already been initialized to run the latest
// source code.
//
// Use Apply for higher level control. Use Init, Build, Push and Deploy
// independently for lower level control.
// Returns final primary route to the Function and any errors.
func (c *Client) Update(ctx context.Context, f Function) (string, Function, error) {
	if !f.Initialized() {
		return "", f, ErrNotInitialized{f.Root}
	}
	var err error
	if f, err = c.Build(ctx, f); err != nil {
		return "", f, err
	}
	if f, err = c.Push(ctx, f); err != nil {
		return "", f, err
	}

	// TODO: change this later when push doesnt return built image.
	// Assign this as c.Push is going to produce the built image (for now) to
	// .Deploy.Image for the deployer -- figure out where to assign .Deploy.Image
	// first, might be just moved above push
	f.Deploy.Image = f.Build.Image

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
// Use Apply for higher level control.  Use Init, Build, Push, Deploy
// independently for lower level control.
// Returns the primary route to the function or error.
func (c *Client) New(ctx context.Context, cfg Function) (string, Function, error) {
	// Always start a concurrent routine listening for context cancellation.
	// On this event, immediately indicate the task is canceling.
	// (this is useful, for example, when a progress listener is mutating
	// stdout, and a context cancelation needs to free up stdout entirely for
	// the status or error from said cancellation.

	var route string
	// Init the path as a new Function
	f, err := c.Init(cfg)
	if err != nil {
		return route, cfg, err
	}

	// Build the now-initialized function
	fmt.Fprintf(os.Stderr, "Building container image\n")
	if f, err = c.Build(ctx, f); err != nil {
		return route, f, err
	}

	// Push the produced function image
	fmt.Fprintf(os.Stderr, "Pushing container image to registry\n")

	if f, err = c.Push(ctx, f); err != nil {
		return route, f, err
	}

	// TODO: change this later when push doesnt return built image.
	// Assign this as c.Push is going to produce the built image (for now) to
	// .Deploy.Image for the deployer -- figure out where to assign .Deploy.Image
	// first, might be just moved above push
	f.Deploy.Image = f.Build.Image

	// Deploy the initialized function, returning its publicly
	// addressible name for possible registration.
	fmt.Fprintf(os.Stderr, "Deploying function to cluster\n")

	if f, err = c.Deploy(ctx, f); err != nil {
		return route, f, err
	}

	// Create an external route to the function
	fmt.Fprintf(os.Stderr, "Creating route to function\n")
	if route, f, err = c.Route(ctx, f); err != nil {
		return route, f, err
	}

	fmt.Fprint(os.Stderr, "Done\n")

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
	if err = ensureRunDataDir(f.Root); err != nil {
		return f, err
	}

	//create a .funcignore file
	if err = ensureFuncIgnore(f.Root); err != nil {
		return f, err
	}

	// Write out the new function's Template files.
	if err = c.Templates().Write(&f); err != nil {
		return f, err
	}

	// Mark the function as having been created, and that it is not to be
	// considered built.
	f.Created = time.Now()
	err = f.Write()
	if err != nil {
		return f, err
	}
	// Load the now-initialized function.
	return NewFunction(oldRoot)
}

type BuildOptions struct {
	Platforms []Platform
}

type BuildOption func(c *BuildOptions)

func BuildWithPlatforms(pp []Platform) BuildOption {
	return func(c *BuildOptions) {
		c.Platforms = pp
	}
}

// Build the function at path. Errors if the function is either unloadable or does
// not contain a populated Image.
func (c *Client) Build(ctx context.Context, f Function, options ...BuildOption) (Function, error) {
	fmt.Fprintf(os.Stderr, "Building function image\n")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// If not logging verbosely, the ongoing progress of the build will not
	// be streaming to stdout, and the lack of activity has been seen to cause
	// users to prematurely exit due to the sluggishness of pulling large images
	if !c.verbose {
		c.printBuildActivity(ctx) // print friendly messages until context is canceled
	}

	// Options for the build task
	oo := BuildOptions{}
	for _, o := range options {
		o(&oo)
	}

	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been specified by user (--image), calculate.
	// Image name is stored on the function for later use by deploy, etc.
	var err error
	if f.Image == "" {
		if f.Build.Image, err = f.ImageName(); err != nil {
			return f, err
		}
	} else {
		f.Build.Image = f.Image
	}

	if err = c.builder.Build(ctx, f, oo.Platforms); err != nil {
		return f, err
	}

	// write .func/built-name as running metadata which is not persisted in yaml
	if err = f.WriteRuntimeBuiltImage(c.verbose); err != nil {
		return f, err
	}

	if err = f.Stamp(); err != nil {
		return f, err
	}

	// TODO: create a status structure and return it here for optional
	// use by the cli for user echo (rather than rely on verbose mode here)
	message := fmt.Sprintf("🙌 Function built: %v", f.Build.Image)
	if runtime.GOOS == "windows" {
		message = fmt.Sprintf("Function built: %v", f.Build.Image)
	}
	fmt.Fprintf(os.Stderr, "%s\n", message)

	return f, err
}

// Scaffold writes a functions's scaffolding to a given path.
// It also updates the included symlink to function source 'f' to point to
// the current function's source.
func (c *Client) Scaffold(ctx context.Context, f Function, dest string) (err error) {
	repo, err := NewRepository("", "") // default (embedded) repository
	if err != nil {
		return
	}
	return scaffolding.Write(dest, f.Root, f.Runtime, f.Invoke, repo.FS())
}

// printBuildActivity is a helper for ensuring the user gets feedback from
// the long task of containerized builds.
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
				fmt.Fprintf(os.Stderr, "%v\n", m[i])
				i++
				i = i % len(m)
			case <-ctx.Done():
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

// Deploy the function at path.
// Errors if the function has not been built unless explicitly instructed
// to ignore this build check.
func (c *Client) Deploy(ctx context.Context, f Function, opts ...DeployOption) (Function, error) {
	deployParams := &DeployParams{skipBuiltCheck: false}
	for _, opt := range opts {
		opt(deployParams)
	}

	go func() {
		<-ctx.Done()
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

	// TODO: gauron99 -- ideally namespace would be determined here to keep consistancy
	// with the Remover but it either creates a cyclic dependency or deployer.namespace
	// is not defined here for it to be complete. Maybe it would be worth to try to
	// do it this way.

	// Deploy a new or Update the previously-deployed function
	fmt.Fprintf(os.Stderr, "⬆️  Deploying function to the cluster\n")
	result, err := c.deployer.Deploy(ctx, f)
	if err != nil {
		fmt.Printf("deploy error: %v\n", err)
		return f, err
	}

	// If Redeployment to NEW namespace was successful -- undeploy dangling Function in old namespace.
	// On forced namespace change (using --namespace flag)
	if f.Namespace != "" && f.Namespace != f.Deploy.Namespace && f.Deploy.Namespace != "" {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Info: Deleting old func in '%s' because the namespace has changed to '%s'\n", f.Deploy.Namespace, f.Namespace)
		}

		// c.Remove removes a Function in f.Deploy.Namespace which removes the OLD Function
		// because its not updated yet (see few lines below)
		err = c.Remove(ctx, f, true)

		// Warn when service is not found and set err to nil to continue. Function's
		// service mightve been manually deleted prior to the subsequent deploy or the
		// namespace is already deleted therefore there is nothing to delete
		if ErrFunctionNotFound != err {
			fmt.Fprintf(os.Stderr, "Warning: Cant undeploy Function in namespace '%s' - service not found. Namespace/Service might be deleted already\n", f.Deploy.Namespace)
			err = nil
		}
		if err != nil {
			return f, err
		}
	}

	// Update the function with the namespace into which the function was
	// deployed
	f.Deploy.Namespace = result.Namespace

	if result.Status == Deployed {
		fmt.Fprintf(os.Stderr, "✅ Function deployed in namespace %q and exposed at URL: \n   %v\n", result.Namespace, result.URL)
	} else if result.Status == Updated {
		fmt.Fprintf(os.Stderr, "✅ Function updated in namespace %q and exposed at URL: \n   %v\n", result.Namespace, result.URL)
	}

	return f, nil
}

// RunPipeline runs a Pipeline to build and deploy the function.
// Returned function contains applicable registry and deployed image name.
func (c *Client) RunPipeline(ctx context.Context, f Function) (Function, error) {
	var err error
	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been specified by user (--image), calculate.
	// Image name is stored on the function for later use by deploy.
	if f.Image != "" {
		// if user specified an image, use it
		f.Deploy.Image = f.Image
	} else if f.Deploy.Image == "" {
		f.Deploy.Image, err = f.ImageName()
		if err != nil {
			return f, err
		}
	}

	// Build and deploy function using Pipeline
	_, f.Deploy.Namespace, err = c.pipelinesProvider.Run(ctx, f)
	if err != nil {
		return f, fmt.Errorf("failed to run pipeline: %w", err)
	}
	return f, nil
}

// ConfigurePAC generates Pipeline resources on the local filesystem,
// on the cluster and also on the remote git provider (ie. GitHub, GitLab or BitBucket repo)
func (c *Client) ConfigurePAC(ctx context.Context, f Function, metadata any) error {
	var err error

	// Default function registry to the client's global registry
	if f.Registry == "" {
		f.Registry = c.registry
	}

	// If no image name has been yet defined (not yet built/deployed), calculate.
	// Image name is stored on the function for later use by deploy, etc.
	if f.Deploy.Image == "" {
		if f.Deploy.Image, err = f.ImageName(); err != nil {
			return err
		}
	}

	// saves image name/registry to function's metadata (func.yaml), and
	// does not explicitly update the last created build stamp
	// (i.e. changes to the function during ConfigurePAC should not cause the
	// next deploy to skip building)
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

type RunOptions struct {
	StartTimeout time.Duration
}

type RunOption func(c *RunOptions)

// RunWithStartTimeout sets a specific timeout for this run request to start.
// If not provided, the client's run timeout (set by default to
// DefaultRunTimeout and configurable via the WithRunTimeout client
// instantiation option) is used.
func RunWithStartTimeout(t time.Duration) RunOption {
	return func(c *RunOptions) {
		c.StartTimeout = t
	}
}

// Run the function whose code resides at root.
// On start, the chosen port is sent to the provided started channel
func (c *Client) Run(ctx context.Context, f Function, options ...RunOption) (job *Job, err error) {

	oo := RunOptions{}
	for _, o := range options {
		o(&oo)
	}

	if !f.Initialized() {
		return nil, fmt.Errorf("can not run an uninitialized function")
	}

	// timeout for this run task.
	timeout := c.startTimeout    // client's global setting is the default
	if f.Run.StartTimeout != 0 { // Function value, if defined, takes precidence
		timeout = f.Run.StartTimeout
	}
	if oo.StartTimeout != 0 { // Highest precidence is an option passed to Run
		timeout = oo.StartTimeout
	}

	// Run the function, which returns a Job for use interacting (at arms length)
	// with that running task (which is likely inside a container process).
	if job, err = c.runner.Run(ctx, f, timeout); err != nil {
		return
	}

	// Return to the caller the effective port, a function to call to trigger
	// stop, and a channel on which can be received runtime errors.
	return job, nil
}

// Describe a function.  Name takes precedence.  If no name is provided,
// the function defined at root is used.
func (c *Client) Describe(ctx context.Context, name string, f Function) (d Instance, err error) {
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

// Remove a function. Name takes precedence. If no name is provided, the
// function defined at root is used if it exists. If calling this directly
// namespace must be provided in .Deploy.Namespace field except when using mocks
// in which case empty namespace is accepted because its existence is checked
// in the sub functions remover.Remove and pipilines.Remove
func (c *Client) Remove(ctx context.Context, cfg Function, deleteAll bool) error {
	functionName := cfg.Name
	functionNamespace := cfg.Deploy.Namespace

	// If name is provided, it takes precedence.
	// Otherwise load the function defined at root.
	if cfg.Name == "" {
		f, err := NewFunction(cfg.Root)
		if err != nil {
			return err
		}
		if !f.Initialized() {
			return fmt.Errorf("function at %v can not be removed unless initialized. Try removing by name", f.Root)
		}
		// take the functions name and namespace and load it as current function
		functionName = f.Name
		functionNamespace = f.Deploy.Namespace
		cfg = f
	}

	// if still empty, get current function's yaml deployed namespace
	if functionNamespace == "" {
		var f Function
		f, err := NewFunction(cfg.Root)
		if err != nil {
			return err
		}
		functionNamespace = f.Deploy.Namespace
	}

	if functionName == "" {
		return ErrNameRequired
	}
	if functionNamespace == "" {
		return ErrNamespaceRequired
	}

	// Delete Knative Service and dependent resources in parallel
	fmt.Fprintf(os.Stderr, "Removing Knative Service: %v in namespace '%v'\n", functionName, functionNamespace)
	errChan := make(chan error)
	go func() {
		errChan <- c.remover.Remove(ctx, functionName, functionNamespace)
	}()

	var errResources error
	if deleteAll {
		fmt.Fprintf(os.Stderr, "Removing Knative Service '%v' and all dependent resources\n", functionName)
		// TODO: might not be necessary
		cfg.Deploy.Namespace = functionNamespace
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

	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("error on push, function has not been built yet")
			return f, err
		}
		return f, err
	}

	imageDigest, err := c.pusher.Push(ctx, f)
	if err != nil {
		return f, err
	}

	// TODO: gauron99 - this is here because of a temporary workaround.
	// f.Build.Image should contain full image name including the sha256 and
	// should be populated earlier BUT because the sha256 is got only on push (here)
	// its populated here. This will eventually be moved to build stage where we get
	// the full image name and its digest right after building
	f.Build.Image = f.ImageNameWithDigest(imageDigest)

	return f, err
}

// ensureRunDataDir creates a .func directory at the given path, and
// registers it as ignored in a .gitignore file.
func ensureRunDataDir(root string) error {
	// Ensure the runtime directory exists
	if err := os.MkdirAll(filepath.Join(root, RunDataDir), os.ModePerm); err != nil {
		return err
	}

	// Update .gitignore
	//
	// Ensure .func is added to .gitignore unless the user explicitly
	// commented out the ignore line for some awful reason.
	// Also creates the .gitignore in the function's root directory if it does
	// not already exist (note that this may not be in the root of the repo
	// if the function is at a subpath of a monorepo)
	filePath := filepath.Join(root, ".gitignore")
	roFile, err := os.Open(filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	defer roFile.Close()
	if !os.IsNotExist(err) { // if no error openeing it
		s := bufio.NewScanner(roFile) // create a scanner
		for s.Scan() {                // scan each line
			if strings.HasPrefix(s.Text(), "# /"+RunDataDir) { // if it was commented
				return nil // user wants it
			}
			if strings.HasPrefix(s.Text(), "#/"+RunDataDir) {
				return nil // user wants it
			}
			if strings.HasPrefix(s.Text(), "/"+RunDataDir) { // if it is there
				return nil // we're done
			}
		}
	}
	// Either .gitignore does not exist or it does not have the ignore
	// directive for .func yet.
	roFile.Close()
	rwFile, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer rwFile.Close()
	if _, err = rwFile.WriteString(`
# Functions use the .func directory for local runtime data which should
# generally not be tracked in source control. To instruct the system to track
# .func in source control, comment the following line (prefix it with '# ').
/.func
`); err != nil {
		return err
	}

	// Flush to disk immediately since this may affect subsequent calculations
	// of the build stamp
	if err = rwFile.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: error when syncing .gitignore. %s", err)
	}
	return nil
}

func ensureFuncIgnore(root string) error {
	filePath := filepath.Join(root, ".funcignore")

	// Check if the file exists
	_, err := os.Stat(filePath)
	if err == nil {
		// File exists, do nothing
		return nil
	}
	if !os.IsNotExist(err) {
		// Some other error occurred when trying to stat the file
		return err
	}

	//file does not exist, create it
	// Open the file for writing only
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the desired string to the file
	_, err = file.WriteString(`
# Use the .funcignore file to exclude files which should not be
# tracked in the image build. To instruct the system not to track
# files in the image build, add the regex pattern or file information
# to this file.
`)
	if err != nil {
		return err
	}
	return nil
}

// Fingerprint the files at a given path.  Returns a hash calculated from the
// filenames and modification timestamps of the files within the given root.
// Also returns a logfile consiting of the filenames and modification times
// which contributed to the hash.
// Intended to determine if there were appreciable changes to a function's
// source code, certain directories and files are ignored, such as
// .git and .func.
// Future updates will include files explicitly marked as ignored by a
// .funcignore.
func Fingerprint(root string) (hash, log string, err error) {
	h := sha256.New()   // Hash builder
	l := bytes.Buffer{} // Log buffer

	err = filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		// Always ignore .func, .git (TODO: .funcignore)
		if info.IsDir() && (info.Name() == RunDataDir || info.Name() == ".git") {
			return filepath.SkipDir
		}
		fmt.Fprintf(h, "%v:%v:", path, info.ModTime().UnixNano())   // Write to the Hasher
		fmt.Fprintf(&l, "%v:%v\n", path, info.ModTime().UnixNano()) // Write to the Log
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil)), l.String(), err
}

// assertEmptyRoot ensures that the directory is empty enough to be used for
// initializing a new function.
func assertEmptyRoot(path string) (err error) {
	// If there exists contentious files (congig files for instance), this function may have already been initialized.
	files, err := contentiousFilesIn(path)
	if err != nil {
		return
	} else if len(files) > 0 {
		return fmt.Errorf("the chosen directory '%v' contains contentious files: %v.  Has the Service function already been created?  Try either using a different directory, deleting the function if it exists, or manually removing the files", path, files)
	}

	// Ensure there are no non-hidden files, and again none of the aforementioned contentious files.
	empty, err := isEffectivelyEmpty(path)
	if err != nil {
		return
	} else if !empty {
		err = errors.New("the directory must be empty of visible files and recognized config files before it can be initialized")
		return
	}
	return
}

// contentiousFiles are files which, if extant, preclude the creation of a
// function rooted in the given directory.
var contentiousFiles = []string{
	FunctionFile,
}

// contentiousFilesIn the given directory
func contentiousFilesIn(path string) (contentious []string, err error) {
	files, err := os.ReadDir(path)
	for _, file := range files {
		for _, name := range contentiousFiles {
			if file.Name() == name {
				contentious = append(contentious, name)
			}
		}
	}
	return
}

// effectivelyEmpty directories are those which have no visible files
func isEffectivelyEmpty(path string) (bool, error) {
	// Check for any non-hidden files
	files, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}

// returns true if the given path contains an initialized function.
func hasInitializedFunction(path string) (bool, error) {
	var err error
	var filename = filepath.Join(path, FunctionFile)

	if _, err = os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err // invalid path or access error
	}
	bb, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	f := Function{}
	if err = yaml.Unmarshal(bb, &f); err != nil {
		return false, err
	}
	if f, err = f.Migrate(); err != nil {
		return false, err
	}
	return f.Initialized(), nil
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

func (n *noopBuilder) Build(ctx context.Context, _ Function, _ []Platform) error { return nil }

// Pusher
type noopPusher struct{ output io.Writer }

func (n *noopPusher) Push(ctx context.Context, f Function) (string, error) { return "", nil }

// Deployer
type noopDeployer struct{ output io.Writer }

func (n *noopDeployer) Deploy(ctx context.Context, f Function) (DeploymentResult, error) {
	return DeploymentResult{Namespace: f.Namespace}, nil
}

// Remover
type noopRemover struct{ output io.Writer }

func (n *noopRemover) Remove(context.Context, string, string) error { return nil }

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

func (n *noopPipelinesProvider) Run(ctx context.Context, _ Function) (string, string, error) {
	return "", "", nil
}
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
