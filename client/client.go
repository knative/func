package client

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v2"
)

const FaasNamespace = "faas"

// Client for a given Service Function.
type Client struct {
	verbose           bool        // print verbose logs
	local             bool        // Run in local-only mode
	internal          bool        // Internal service flag (no public route)
	name              string      // Service function DNS address (configurable)
	root              string      // root path of function on which to operate
	domainSearchLimit int         // max dirs to recurse up when deriving domain
	dnsProvider       DNSProvider // Provider of DNS services
	initializer       Initializer // Creates initial local function implementation
	builder           Builder     // Builds a runnable image from function source
	pusher            Pusher      // Pushes a built image to a registry
	deployer          Deployer    // Deploys a Service Function
	updater           Updater     // Updates a deployed Service Function
	runner            Runner      // Runs the function locally
	remover           Remover     // Removes remote services
	lister            Lister      // Lists remote services
}

// ConfigFileName is an optional file checked for in the function root.
const ConfigFileName = ".faas.yaml"

// Config object which provides another mechanism for overriding client static
// defaults.  Applied prior to the WithX options, such that the options take
// precedence if they are provided.
type Config struct {
	// Name specifies the name to be used for this function. As a config option,
	// this value, if provided, takes precidence over the path-derived name but
	// not over the Option WithName, if provided.
	Name string `yaml:"name"`

	// Add new values to the applyConfig function as necessary.
}

// DNSProvider exposes DNS services necessary for serving the Service Function.
type DNSProvider interface {
	// Provide the given name by routing requests to address.
	Provide(name, address string)
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

// Option defines a function which when passed to the Client constructor optionally
// mutates private members at time of instantiation.
type Option func(*Client)

// WithVerbose toggles verbose logging.
func WithVerbose(v bool) Option {
	return func(c *Client) {
		c.verbose = v
	}
}

// WithName sets the explicit name for the Service Function, disabling
// name inference from path.
func WithName(name string) Option {
	return func(c *Client) {
		c.name = name
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

// WithDNSProvider proivdes a DNS provider implementation for registering the
// effective DNS name which is either explicitly set via WithName or is derived
// from the root path.
func WithDNSProvider(provider DNSProvider) Option {
	return func(c *Client) {
		c.dnsProvider = provider
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

// New client for a function service rooted at the given directory (default .) or
// that explicitly set via the option.  Will fail if the directory already contains
// config files or other non-hidden files.
func New(root string, options ...Option) (c *Client, err error) {
	if root == "" {
		root = "."
	}
	// Instantiate client with static defaults.
	c = &Client{
		root:              root,
		dnsProvider:       &noopDNSProvider{output: os.Stdout},
		initializer:       &noopInitializer{output: os.Stdout},
		builder:           &noopBuilder{output: os.Stdout},
		pusher:            &noopPusher{output: os.Stdout},
		deployer:          &noopDeployer{output: os.Stdout},
		updater:           &noopUpdater{output: os.Stdout},
		runner:            &noopRunner{output: os.Stdout},
		remover:           &noopRemover{output: os.Stdout},
		domainSearchLimit: -1, // no recursion limit deriving domain by default.
	}

	// Apply config file in the given path if it exists, overriding static defaults above.
	if err = applyConfig(c, c.root); err != nil {
		return
	}

	// Apply passed options, which take ultimate precidence.
	for _, o := range options {
		o(c)
	}

	// Working Directory
	// Convert the specified root to an absolute path.  If no root is provided,
	// the root is the current working directory.
	c.root, err = filepath.Abs(c.root)
	if err != nil {
		return
	}

	// Service Name
	// If not explicity set via the WithName option, we attempt to derive the
	// name from the effective root path.
	if c.name == "" {
		c.name = pathToDomain(c.root, c.domainSearchLimit)
	}
	if c.name == "" {
		return c, errors.New("Function name must be provided or be derivable from path.")
	}

	return
}

// SetLocal mode (skips push, deploy, etc.)
func (c *Client) SetLocal(local bool) {
	c.local = local
}

// SetInternal mode skips creation of a publicly accessible route to the
// function, making it available only to other services in the cluster.
func (c *Client) SetInternal(internal bool) {
	c.internal = internal
}

// Create a service function
func (c *Client) Create(language string) (err error) {
	// Language is required
	// Whether or not the given language is supported is dependant on
	// the implementation of the initializer.
	if language == "" {
		return errors.New("language not specified")
	}

	// Assert the root does not contain contentious hidden files (configs).
	var files []string
	if files, err = contentiousFilesIn(c.root); err != nil {
		return
	} else if len(files) > 0 {
		err = errors.New(fmt.Sprintf("The directory has extant faas config files.  Has the service funciton already been created?  Either use a different directory, delete the service function if it exists, or remove the files manually: %v", files))
		return
	}

	// Assert the local directory is empty of all non-hidden files/dirs, and of
	// the hidden files .appsody-config.yaml and .faas.yaml.
	var empty bool
	if empty, err = isEffectivelyEmpty(c.root); err != nil {
		return
	} else if !empty {
		err = errors.New("The directory contains visible and/or recognized config files.")
		return
	}

	// Initialize the specified root with a function template.
	// TODO: detect extant and abort.
	if err = c.initializer.Initialize(c.name, language, c.root); err != nil {
		return
	}

	// Write the effective config once initialization was successful.
	if err = writeConfig(c); err != nil {
		return
	}

	// Build the now-initialized service function
	image, err := c.builder.Build(c.name, c.root)
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

	// Deploy the initialized service function, returning its publicly
	// addressible name for possible registration.
	address, err := c.deployer.Deploy(c.name, image)
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
	c.dnsProvider.Provide(c.name, address)
	fmt.Printf("%v\n", address)

	return
}

// Update a previously created service function.
func (c *Client) Update() (err error) {

	// TODO: detect and error if `create` was never run, failed, or the
	// service is othewise un-updatable.

	// Build an image from the current state of the service function's codebase.
	image, err := c.builder.Build(c.name, c.root)
	if err != nil {
		return
	}

	// Push the image for the named service to the configured registry
	if err = c.pusher.Push(image); err != nil {
		return
	}

	// Update the previously-deployed service function, returning its publicly
	// addressible name for possible registration.
	return c.updater.Update(c.name, image)
}

// Run the function whose code resides at root.
func (c *Client) Run() error {
	// delegate to concrete implementation of runner entirely.
	return c.runner.Run(c.root)
}

// List currently deployed service functions.
func (c *Client) List() ([]string, error) {
	// delegate to concrete implementation of lister entirely.
	return c.lister.List()
}

// Remove a function from remote, bringing the service funciton
// to the same state as if it had been created --local only.
// Name is the presently configured client's name, which was
// either derived from the path, specified by the extant config,
// or provided as an option (in ascending precedence).
func (c *Client) Remove() error {
	// delegate to concrete implementation of remover entirely.
	return c.remover.Remove(c.name)
}

// Convert a path to a domain.
// Searches up the path string until a domain (TLD+1) is detected.
// Subdirectories are considered subdomains.
// Ex: Path:    "/home/users/jane/src/example.com/admin/www"
//     Returns: "www.admin.example.com"
// maxLevels is the number of directories to walk upwards beyond the current
// directory to determine domain (i.e. current directory is always considered.
// Zero indicates only consider last path element.)
func pathToDomain(path string, maxLevels int) string {
	var (
		// parts of the path, separated by os separator
		parts = strings.Split(path, string(os.PathSeparator))

		// subdomains derived from the path
		subdomains []string

		// domain derived from the path
		domain string
	)

	// Loop over parts from back to front (recursing upwards), building
	// optional subdomains until a root domain (TLD+1) is detected.
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]

		// Support limited recursion
		// Tests, for instance, need to be allowed to reliably fail by having their
		// recursion contained within ./testdata if recursion is set to -1, there
		// is no limit.  0 indicates only the current directory is considered.
		iteration := len(parts) - 1 - i
		if maxLevels >= 0 && iteration > maxLevels {
			break
		}

		// Detect TLD+1
		// If the current directory has a valid TLD plus one, it is a match.
		// This is determined by using the public suffices list, which includes
		// both ICANN managed TLDs as well as an extended list (matching, for
		// instance 'cluster.local')
		if suffix, _ := publicsuffix.EffectiveTLDPlusOne(part); suffix != "" {
			domain = part
			break // no directories above the nearest TLD+1 should be considered.
		}

		// Skip blanks
		// Path elements which are blank, such as in the case of a trailing slash
		// are ignored and the recursion continues, effectively collapsing ex: '//'.
		if part == "" {
			continue
		}

		// Build subdomain
		// Each path element which appears before the TLD+1 is a subdomain.
		// ex: '/home/users/jane/src/example.com/us-west-2/admin/www' creates the
		// subdomain []string{'www', 'admin', 'us-west-2'}
		subdomains = append(subdomains, part)
	}

	// Unable to derive domain
	// If the entire path was searched, but no parts matched a TLD+1, the domain
	// will be blank.  In this case, the path was insufficient to derive a domain
	// ex "/home/users/jane/src/test" contains no TLD, thus the final domain must
	// be explicitly provided.
	if domain == "" {
		return ""
	}

	// Prepend subdomains
	// If the path was a subdirectory within a TLD+1, these sudbomains
	// are prepended to the TLD+1 to create the final domain.
	// ex: '/home/users/jane/src/example.com/us-west-2/admin/www' yields
	// www.admin.use-west-2.example.com
	if len(subdomains) > 0 {
		subdomains = append(subdomains, domain)
		return strings.Join(subdomains, ".")
	}

	return domain
}

// Manual implementations (noops) of required interfaces.
// In practice, the user of this client package (for example the CLI) will
// provide a concrete implementation for all of the interfaces.  For testing or
// development, however, it is usefule that they are defaulted to noops and
// provded only when necessary.  Unit tests for the concrete implementations
// serve to keep the core logic here separate from the imperitive.
// -----------------------------------------------------

type noopDNSProvider struct{ output io.Writer }

func (p *noopDNSProvider) Provide(name, address string) {
	fmt.Fprintln(p.output, "skipping DNS update: client not initialized WithDNSProvider")
}

type noopInitializer struct{ output io.Writer }

func (i *noopInitializer) Initialize(name, language, root string) error {
	fmt.Fprintln(i.output, "skipping initialize: client not initialized WithInitializer")
	return nil
}

type noopBuilder struct{ output io.Writer }

func (i *noopBuilder) Build(name, root string) (image string, err error) {
	fmt.Fprintln(i.output, "skipping build: client not initialized WithBuilder")
	return "", nil
}

type noopPusher struct{ output io.Writer }

func (i *noopPusher) Push(image string) error {
	fmt.Fprintln(i.output, "skipping push: client not initialized WithPusher")
	return nil
}

type noopDeployer struct{ output io.Writer }

func (i *noopDeployer) Deploy(name, image string) (string, error) {
	fmt.Fprintln(i.output, "skipping deploy: client not initialized WithDeployer")
	return "", nil
}

type noopUpdater struct{ output io.Writer }

func (i *noopUpdater) Update(name, image string) error {
	fmt.Fprintln(i.output, "skipping deploy: client not initialized WithDeployer")
	return nil
}

type noopRunner struct{ output io.Writer }

func (i *noopRunner) Run(root string) error {
	fmt.Fprintln(i.output, "skipping run: client not initialized WithRunner")
	return nil
}

type noopRemover struct{ output io.Writer }

func (i *noopRemover) Remove(name string) error {
	fmt.Fprintln(i.output, "skipping remove: client not initialized WithRemover")
	return nil
}

// contentiousFiles are files which, if extant, preclude the creation of a
// service function rooted in the given directory.
var contentiousFiles = []string{
	".faas.yaml",
	".appsody-config.yaml",
}

// contentiousFilesIn the given directoy
func contentiousFilesIn(dir string) (contentious []string, err error) {
	files, err := ioutil.ReadDir(dir)
	for _, file := range files {
		for _, name := range contentiousFiles {
			if file.Name() == name {
				contentious = append(contentious, name)
			}
		}
	}
	return
}

// effectivelyEmpty directories are those which have no visible files,
// and no explicitly enumerated contentious files.
func isEffectivelyEmpty(dir string) (empty bool, err error) {
	var contentious []string
	if contentious, err = contentiousFilesIn(dir); len(contentious) > 0 {
		return
	}
	files, err := ioutil.ReadDir(dir)
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			return
		}
	}
	return true, nil
}

// Apply the config, if it exists, to the client.
// if an entry exists in the config file and is empty, this is interpreted as
// the intent to zero-value that field.
func applyConfig(c *Client, root string) error {
	// abort if the config file does not exist.
	filename := filepath.Join(root, ConfigFileName)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	}

	// Read in as bytes
	bb, err := ioutil.ReadFile(filepath.Join(root, ConfigFileName))
	if err != nil {
		return err
	}

	// Create a config with defaults set to the current value of the Client object.
	// These gymnastics are necessary because we want the Client's members to be
	// private to disallow mutation post instantiation, and thus they are unavailable
	// to be set automatically
	cfg := newConfig(c)

	// Decode yaml, overriding values in the config if they were defined in the yaml.
	if err := yaml.Unmarshal(bb, &cfg); err != nil {
		return err
	}

	// Apply the config to the client object, which effectiely writes back the default
	// if it was not defined in the yaml.
	c.name = cfg.Name

	// NOTE: cleverness < clarity

	return nil
}

// newConfig creates a config object from a client, effectively exporting mutable
// fields for the config file while preserving the immutability of the client
// post-instantiation.
func newConfig(c *Client) Config {
	return Config{
		Name: c.name,
	}
}

// writeConfig out to disk.
func writeConfig(c *Client) (err error) {
	var (
		cfg     = newConfig(c)
		cfgFile = filepath.Join(c.root, ConfigFileName)
		bb      []byte
	)
	if bb, err = yaml.Marshal(&cfg); err != nil {
		return
	}
	return ioutil.WriteFile(cfgFile, bb, 0644)
}
