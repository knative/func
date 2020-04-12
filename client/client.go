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
)

// Client for a given Service Function.
type Client struct {
	verbose           bool        // print verbose logs
	local             bool        // Run in local-only mode
	name              string      // Service function DNS address
	root              string      // root path of function on which to operate
	domainSearchLimit int         // max dirs to recurse up when deriving domain
	dnsProvider       DNSProvider // Provider of DNS services
	initializer       Initializer // Creates initial local function implementation
	builder           Builder     // Builds a runnable image from function source
	pusher            Pusher      // Pushes a built image to a registry
	deployer          Deployer    // Deploys a Service Function
	runner            Runner      // Runs the function locally
	remover           Remover     // Removes remote services.
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
	// Build a function service of the given name with source located at path.
	// returns the image name built.
	Build(name, path string) (image string, err error)
}

// Pusher of function image to a registry.
type Pusher interface {
	// Push the image of the function service.
	Push(image string) error
}

// Deployer of function source to running status.
type Deployer interface {
	// Deploy a service function of given name, using given backing image.
	Deploy(name, image string) (address string, err error)
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

// WithRoot explicitly sets the root effective path for the client, which is used
// to write new Service Function shell files, for determinging effective DNS
// name (unless WithName was explicitly provided), for reading and writing
// config, etc.  By default this is the current working directory of the process.
func WithRoot(path string) Option {
	return func(c *Client) {
		c.root = path
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

// New client for a function service rooted at the current working directory or
// that explicitly set via the option.  Will fail if the directory already contains
// config files or other non-hidden files.
func New(options ...Option) (c *Client, err error) {
	// Client with defaults overridden by optional parameters
	c = &Client{
		domainSearchLimit: -1, // no recursion limit deriving domain by default.
		dnsProvider:       &manualDNSProvider{output: os.Stdout},
		initializer:       &manualInitializer{output: os.Stdout},
		builder:           &manualBuilder{output: os.Stdout},
		pusher:            &manualPusher{output: os.Stdout},
		deployer:          &manualDeployer{output: os.Stdout},
		runner:            &manualRunner{output: os.Stdout},
		remover:           &manualRemover{output: os.Stdout},
	}
	for _, o := range options {
		o(c)
	}

	// Convert the specified root to an absolute path.
	// If no root is provided, the root is the current working directory.
	c.root, err = filepath.Abs(c.root)
	if err != nil {
		return
	}

	// Derive name
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

	// TODO
	// Dervive the cluster address of the service.
	// Derive the public domain of the service from the directory path.
	c.dnsProvider.Provide(c.name, address)

	// Associate the public domain to the cluster-defined address.
	return
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

// Run the function whose code resides at root.
func (c *Client) Run() error {
	// delegate to concrete implementation of runner entirely.
	return c.runner.Run(c.root)
}

// Remove a function from remote, bringing the service funciton
// to the same state as if it had been created --local only.
// Name is optional, as the presently associated service function
// is inferred, but a client is allowed to remove any service
// function for which the user has permission to remove, as this
// is used for repairing broken local->remote associations.
func (c *Client) Remove(name string) error {
	if name == "" {
		name = c.name
	}
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

type manualDNSProvider struct {
	output io.Writer
}

func (p *manualDNSProvider) Provide(name, address string) {
	if address == "" {
		address = "[manually configured address]"
	}
	fmt.Fprintf(p.output, "Please manually configure '%v' to route requests to '%v' \n", name, address)
}

type manualInitializer struct {
	output io.Writer
}

func (i *manualInitializer) Initialize(name, language, root string) error {
	fmt.Fprintf(i.output, "Please create a base function for '%v' (language '%v') at path '%v'\n", name, language, root)
	return nil
}

type manualBuilder struct {
	output io.Writer
}

func (i *manualBuilder) Build(name, root string) (image string, err error) {
	fmt.Fprintf(i.output, "Please manually build image for '%v' using code at '%v'\n", name, root)
	return "", nil
}

type manualPusher struct {
	output io.Writer
}

func (i *manualPusher) Push(image string) error {
	fmt.Fprintf(i.output, "Please manually push image '%v'\n", image)
	return nil
}

type manualDeployer struct {
	output io.Writer
}

func (i *manualDeployer) Deploy(name, image string) (string, error) {
	fmt.Fprintf(i.output, "Please manually deploy '%v'\n", name)
	return "", nil
}

type manualRunner struct {
	output io.Writer
}

func (i *manualRunner) Run(root string) error {
	fmt.Fprintf(i.output, "Please manually run using code at '%v'\n", root)
	return nil
}

type manualRemover struct {
	output io.Writer
}

func (i *manualRemover) Remove(name string) error {
	fmt.Fprintf(i.output, "Please manually remove service '%v'\n", name)
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
