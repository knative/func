package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"gopkg.in/yaml.v2"
	fnlabels "knative.dev/func/pkg/k8s/labels"
	"knative.dev/pkg/ptr"
)

const (
	// FunctionFile is the file used for the serialized form of a function.
	FunctionFile = "func.yaml"

	// RunDataDir holds transient runtime metadata
	// By default it is excluded from source control.
	RunDataDir       = ".func"
	RunDataLocalFile = "local.yaml"

	// BuiltHash is a name of a file that holds hash of built Function in runtime
	// metadata dir (RunDataDir)
	BuiltHash = "built-hash"

	// BuiltImage is a name of a file that holds name of built image in runtime
	// metadata dir (RunDataDir)
	BuiltImage = "built-image"
)

// Local represents the transient runtime metadata which
// is only relevant to the local copy of the function
type Local struct {
	// Remote indicates the deployment (and possibly build) process are to
	// be triggered in a remote environment rather than run locally.
	Remote bool `yaml:"remote,omitempty"`
}

// Function
type Function struct {
	// SpecVersion at which this function is known to be compatible.
	// More specifically, it is the highest migration which has been applied.
	// For details see the .Migrated() and .Migrate() methods.
	SpecVersion string `yaml:"specVersion"` // semver format

	// Root on disk at which to find/create source and config files.
	Root string `yaml:"-"`

	// Name of the function.
	Name string `yaml:"name,omitempty" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`

	// Domain of the function optionally specifies the domain to use as the
	// route of the function. By default the cluster's default will be used.
	// Note that the value defined here must be one which the cluster is
	// configured to recognize, or this will have no effect and the cluster
	// default will be applied.  This value shuld therefore ideally be
	// validated by the client.
	Domain string `yaml:"domain,omitempty"`

	// Runtime is the language plus context.  nodejs|go|quarkus|rust etc.
	Runtime string `yaml:"runtime,omitempty"`

	// Template for the function.
	Template string `yaml:"-"`

	// Registry at which to store interstitial containers, in the form
	// [registry]/[user].
	Registry string `yaml:"registry,omitempty"`

	// Image is the full OCI image tag in form:
	//   [registry]/[namespace]/[name]:[tag]
	// example:
	//   quay.io/alice/my.function.name
	// Registry is optional and is defaulted to DefaultRegistry
	// example:
	//   alice/my.function.name
	// If Image is provided, it overrides the default of concatenating
	// "Registry+Name:latest" to derive the Image.
	Image string `yaml:"image,omitempty"`

	// Namespace in which to deploy the Function
	Namespace string `yaml:"namespace,omitempty"`

	// Created time is the moment that creation was successfully completed
	// according to the client which is in charge of what constitutes being
	// fully "Created" (aka initialized)
	Created time.Time `yaml:"created"`

	// Invoke defines hints for use when invoking this function.
	// See Client.Invoke for usage.
	Invoke string `yaml:"invoke,omitempty" jsonschema:"enum=http,enum=cloudevent"`

	// Build defines the build properties for a function
	Build BuildSpec `yaml:"build,omitempty"`

	// Run defines the runtime properties for a function
	Run RunSpec `yaml:"run,omitempty"`

	// Deploy defines the deployment properties for a function
	Deploy DeploySpec `yaml:"deploy,omitempty"`

	Local Local `yaml:"-"`
}

// KnativeSubscription
type KnativeSubscription struct {
	Source  string            `yaml:"source"`
	Filters map[string]string `yaml:"filters,omitempty"`
}

// BuildSpec
type BuildSpec struct {
	// Git stores information about an optionally associated git repository.
	Git Git `yaml:"git,omitempty"`

	// BuilderImages define optional explicit builder images to use by
	// builder implementations in leau of the in-code defaults.  They key
	// is the builder's short name.  For example:
	// builderImages:
	//   pack: example.com/user/my-pack-node-builder
	//   s2i: example.com/user/my-s2i-node-builder
	BuilderImages map[string]string `yaml:"builderImages,omitempty"`

	// Optional list of buildpacks to use when building the function
	Buildpacks []string `yaml:"buildpacks,omitempty"`

	// Builder is the name of the subsystem that will complete the underlying
	// build (pack, s2i, etc)
	Builder string `yaml:"builder,omitempty" jsonschema:"enum=pack,enum=s2i"`

	// Build Env variables to be set
	BuildEnvs Envs `yaml:"buildEnvs,omitempty"`

	// PVCSize specifies the size of persistent volume claim used to store function
	// when using deployment and remote build process (only relevant when Remote is true).
	PVCSize string `yaml:"pvcSize,omitempty"`

	// RemoteStorageClass specifies the storage class to use for the volume used
	// on-cluster during when built remotely.
	RemoteStorageClass string `yaml:"remoteStorageClass,omitempty"`

	// Image stores last built image name NOT in func.yaml, but instead
	// in .func/built-image
	Image string `yaml:"-"`

	// Mounts used in build phase. This is useful in particular for paketo bindings.
	Mounts []MountSpec `yaml:"volumes,omitempty"`
}

type MountSpec struct {
	// Path on the local machine
	Source string `yaml:"hostPath"`
	// Path in the build container
	Destination string `yaml:"path"`
}

// RunSpec
type RunSpec struct {
	// List of volumes to be mounted to the function
	Volumes []Volume `yaml:"volumes,omitempty"`

	// Env variables to be set
	Envs Envs `yaml:"envs,omitempty"`

	// StartTimeout specifies that this function should have a custom timeout
	// when starting. This setting is currently respected by the host runner,
	// with containerized docker runner and deployed Knative service integration
	// in development.
	StartTimeout time.Duration `yaml:"startTimeout,omitempty"`
}

// DeploySpec
type DeploySpec struct {
	// Namespace into which the function was deployed on supported platforms.
	Namespace string `yaml:"namespace,omitempty"`

	// Image is the deployed image including sha256
	Image string `yaml:"image,omitempty"`

	// Map containing user-supplied annotations
	// Example: { "division": "finance" }
	Annotations map[string]string `yaml:"annotations,omitempty"`

	// Options to be set on deployed function (scaling, etc.)
	Options Options `yaml:"options,omitempty"`

	// Map of user-supplied labels
	Labels []Label `yaml:"labels,omitempty"`

	// Health endpoints specified by the language pack
	HealthEndpoints HealthEndpoints `yaml:"healthEndpoints,omitempty"`

	// ServiceAccountName is the name of the service account used for the
	// function pod. The service account must exist in the namespace to
	// succeed.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	ServiceAccountName string `yaml:"serviceAccountName,omitempty"`

	Subscriptions []KnativeSubscription `yaml:"subscriptions,omitempty"`
}

// HealthEndpoints specify the liveness and readiness endpoints for a Runtime
type HealthEndpoints struct {
	Liveness  string `yaml:"liveness,omitempty"`
	Readiness string `yaml:"readiness,omitempty"`
}

// BuildConfig defines builders and buildpacks
type BuildConfig struct {
	Buildpacks    []string          `yaml:"buildpacks,omitempty"`
	BuilderImages map[string]string `yaml:"builderImages,omitempty"`
}

// NewFunctionWith defaults as provided.
func NewFunctionWith(defaults Function) Function {
	// Deprecatded:  these defaults should be used directly from their
	// in-code static defaults, config, etc. A function struct is used to hold
	// overrides (eg. use PVCSize X instead of the default), and to record the
	// results of operations (eg. the function was deployed with image Y).
	if defaults.SpecVersion == "" {
		defaults.SpecVersion = LastSpecVersion()
	}
	if defaults.Template == "" {
		defaults.Template = DefaultTemplate
	}
	if defaults.Build.BuilderImages == nil {
		defaults.Build.BuilderImages = make(map[string]string)
	}
	if defaults.Deploy.Annotations == nil {
		defaults.Deploy.Annotations = make(map[string]string)
	}
	return defaults
}

// NewFunction from a given path.
// Invalid paths, or no function at path are errors.
// Syntactic errors are returned immediately (yaml unmarshal errors).
// Functions which are syntactically valid are also then logically validated.
// Functions from earlier versions are brought up to current using migrations.
// Migrations are run prior to validators such that validation can omit
// concerning itself with backwards compatibility. Migrators must therefore
// selectively consider the minimal validation necessary to enable migration.
func NewFunction(root string) (f Function, err error) {
	f.Build.BuilderImages = make(map[string]string)
	f.Deploy.Annotations = make(map[string]string)

	// Path defaults to current working directory, and if provided explicitly
	// Path must exist and be a directory
	if root == "" {
		if root, err = os.Getwd(); err != nil {
			return
		}
	}
	f.Root = root // path is not persisted, as this is the purview of the FS

	// Path must exist and be a directory
	fd, err := os.Stat(root)
	if err != nil {
		return f, err
	}
	if !fd.IsDir() {
		return f, fmt.Errorf("function path must be a directory")
	}

	// If no func.yaml in directory, return the default function which will
	// have f.Initialized() == false
	var filename = filepath.Join(root, FunctionFile)
	if _, err = os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}

	// Path is valid and func.yaml exists: load it
	bb, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	var functionMarshallingError error
	var functionMigrationError error
	if marshallingErr := yaml.Unmarshal(bb, &f); marshallingErr != nil {
		functionMarshallingError = formatUnmarshalError(marshallingErr) // human-friendly unmarshalling errors
	}
	if f, err = f.Migrate(); err != nil {
		functionMigrationError = err
	}
	// Only if migration fail return errors to the user. include marshalling error if present
	if functionMigrationError != nil {
		//returning both  migrations and marshalling errors to the user
		errorText := "Error: \n"
		if functionMarshallingError != nil {
			errorText += "Marshalling: " + functionMarshallingError.Error()
		}
		errorText += "\n" + "Migration: " + functionMigrationError.Error()
		return Function{}, errors.New(errorText)
	}

	f.Local, err = f.newLocal()
	if err != nil {
		return
	}
	// ---- LOCAL SETTINGS - STUFF NOT IN FUNC.YAML ---- //

	f.Build.Image, err = f.getLastBuiltImage()

	return
}

// Validate function is logically correct, returning a bundled, and quite
// verbose, formatted error detailing any issues.
func (f Function) Validate() error {
	if f.Root == "" {
		return errors.New("function root path is required")
	}

	var ctr int
	errs := [][]string{
		validateVolumes(f.Run.Volumes),
		ValidateBuildEnvs(f.Build.BuildEnvs),
		ValidateEnvs(f.Run.Envs),
		validateOptions(f.Deploy.Options),
		ValidateLabels(f.Deploy.Labels),
		validateGit(f.Build.Git),
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("'%v' contains errors:", FunctionFile))

	for _, ee := range errs {
		if len(ee) > 0 {
			b.WriteString("\n") // Precede each group of errors with a linebreak
		}
		for _, e := range ee {
			ctr++
			b.WriteString("\t" + e)
		}
	}

	if ctr == 0 {
		return nil // Return nil if there were no validation errors.
	}

	return errors.New(b.String())
}

var envPattern = regexp.MustCompile(`^{{\s*(\w+)\s*:(\w+)\s*}}$`)

// Interpolate Env slice
// Values with no special format are preserved as simple values.
// Values which do include the interpolation format (begin with {{) but are not
// keyed as "env" are also preserved as is.
// Values properly formatted as {{ env:NAME }} are interpolated (substituted)
// with the value of the local environment variable "NAME", and an error is
// returned if that environment variable does not exist.
func Interpolate(ee []Env) (map[string]string, error) {
	envs := make(map[string]string, len(ee))
	for _, e := range ee {
		// Assert non-nil name.
		if e.Name == nil {
			return envs, errors.New("env name may not be nil")
		}
		// Nil value indicates the resultant map should not include this env var.
		if e.Value == nil {
			continue
		}
		k, v := *e.Name, *e.Value

		// Simple Values are preserved.
		// If not prefixed by {{, no interpolation required (simple value)
		if !strings.HasPrefix(v, "{{") {
			envs[k] = v // no interpolation required.
			continue
		}

		// Values not matching the interpolation pattern are preserved.
		// If not in the form "{{ env:XYZ }}" then return the value as-is for
		//                     0  1   2   3
		// possible match and interpolation in different ways.
		parts := envPattern.FindStringSubmatch(v)
		if len(parts) <= 2 || parts[1] != "env" {
			envs[k] = v
			continue
		}

		// Properly formatted local env var references are interpolated.
		localName := parts[2]
		localValue, ok := os.LookupEnv(localName)
		if !ok {
			return envs, fmt.Errorf("expected environment variable '%v' not found", localName)
		}
		envs[k] = localValue
	}
	return envs, nil
}

// nameFromPath returns the default name for a function derived from a path.
// This consists of the last directory in the given path, if derivable (empty
// paths, paths consisting of all slashes, etc. return the zero value "")
func nameFromPath(path string) string {
	pathParts := strings.Split(strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator))
	return pathParts[len(pathParts)-1]
	/* the above may have edge conditions as it assumes the trailing value
	 * is a directory name.  If errors are encountered, we _may_ need to use the
	 * inbuilt logic in the std lib and either check if the path indicated is a
	 * directory (appending slash) and then run:
					 base := filepath.Base(filepath.Dir(path))
					 if base == string(os.PathSeparator) || base == "." {
									 return "" // Consider it underivable: string zero value
					 }
					 return base
	*/
}

// Write Function struct (metadata) to Disk at f.Root
func (f Function) Write() (err error) {
	// Skip writing (and dirtying the work tree) if there were no modifications.
	f1, _ := NewFunction(f.Root)
	if reflect.DeepEqual(f, f1) {
		return
	}

	// Do not write invalid functions
	if err = f.Validate(); err != nil {
		return
	}

	// Write
	var bb []byte
	if bb, err = yaml.Marshal(&f); err != nil {
		return
	}
	// TODO: open existing file for writing, such that existing permissions
	// are preserved?
	rwFile, err := os.OpenFile(filepath.Join(f.Root, FunctionFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer rwFile.Close()

	tagVersion := f.SpecVersion

	// Write schema header
	schemaHeader := fmt.Sprintf(`# $schema: https://raw.githubusercontent.com/knative/func/refs/tags/v%s/schema/func_yaml-schema.json
# yaml-language-server: $schema=https://raw.githubusercontent.com/knative/func/refs/tags/v%s/schema/func_yaml-schema.json
`, tagVersion, tagVersion)

	if _, err = rwFile.WriteString(schemaHeader); err != nil {
		return err
	}

	// Write function data
	if _, err = rwFile.Write(bb); err != nil {
		return err
	}

	// Write local settings
	err = ensureRunDataDir(f.Root)
	if err != nil {
		return
	}
	if bb, err = yaml.Marshal(&f.Local); err != nil {
		return
	}
	localConfigPath := filepath.Join(f.Root, RunDataDir, RunDataLocalFile)

	if err = os.WriteFile(localConfigPath, bb, 0644); err != nil {
		return
	}

	// Write built image to .func
	err = f.WriteRuntimeBuiltImage(false)
	return
}

type stampOptions struct{ journal bool }
type stampOption func(o *stampOptions)

// WithStampJournaling is a Stamp option which causes the stamp logfile
// to be created with a timestamp prefix.  This has the effect of creating
// a stamp journal, useful for debugging.  The default behavior is to only
// retain the most recent log file as "built.log".
func WithStampJournal() stampOption {
	return func(o *stampOptions) {
		o.journal = true
	}
}

// Stamp a function as being built.
//
// This is a performance optimization used when updates to the
// function are known to have no effect on its built container.  This
// stamp is checked before certain operations, and if it has been updated,
// the build can be skuipped.  If in doubt, just use .Write only.
//
// Updates the build stamp at ./func/built (and the log
// at .func/built.log) to reflect the current state of the filesystem.
// Note that the caller should call .Write first to flush any changes to the
// function in-memory to the filesystem prior to calling stamp.
//
// The runtime data directory .func is created in the function root if
// necessary.
func (f Function) Stamp(oo ...stampOption) (err error) {
	options := &stampOptions{}
	for _, o := range oo {
		o(options)
	}
	if err = ensureRunDataDir(f.Root); err != nil {
		return
	}

	// Cacluate the hash and a logfile of what comprised it
	var hash, log string
	if hash, log, err = Fingerprint(f.Root); err != nil {
		return
	}

	// Write out the hash
	if err = os.WriteFile(filepath.Join(f.Root, RunDataDir, BuiltHash), []byte(hash), os.ModePerm); err != nil {
		return
	}

	// Write out the logfile, optionally timestamped for retention.
	logfileName := "built.log"
	if options.journal {
		logfileName = timestamp(logfileName)
	}
	logfile, err := os.Create(filepath.Join(f.Root, RunDataDir, logfileName))
	if err != nil {
		return
	}
	defer logfile.Close()
	_, err = fmt.Fprintln(logfile, log)
	return
}

// timestamp returns the given string prefixed with a microsecond-precision
// timestamp followed by a dot.
// YYYYMMDDHHMMSS.$nanosecond.$s
func timestamp(s string) string {
	t := time.Now()
	return fmt.Sprintf("%s.%09d.%s", t.Format("20060102150405"), t.Nanosecond(), s)
}

// Initialized returns if the function has been initialized.
// Any errors are considered failure (invalid or inaccessible root, config file, etc).
func (f Function) Initialized() bool {
	return !f.Created.IsZero()
}

// LabelsMap combines default labels with the labels slice provided.
// It will the resulting slice with ValidateLabels and return a key/value map.
//   - key: EXAMPLE1                            # Label directly from a value
//     value: value1
//   - key: EXAMPLE2                            # Label from the local ENV var
//     value: {{ env:MY_ENV }}
func (f Function) LabelsMap() (map[string]string, error) {
	defaultLabels := []Label{
		{
			Key:   ptr.String(fnlabels.FunctionNameKey),
			Value: ptr.String(f.Name),
		},
		{
			Key:   ptr.String(fnlabels.FunctionRuntimeKey),
			Value: ptr.String(f.Runtime),
		},
	}

	labels := append(defaultLabels, f.Deploy.Labels...)
	if err := ValidateLabels(labels); len(err) != 0 {
		return nil, errors.New(strings.Join(err, " "))
	}

	l := map[string]string{}
	for _, label := range labels {
		if label.Value == nil {
			l[*label.Key] = ""
		} else {
			if strings.HasPrefix(*label.Value, "{{") {
				// env variable format is validated above in ValidateLabels
				match := regLocalEnv.FindStringSubmatch(*label.Value)
				l[*label.Key] = os.Getenv(match[1])
			} else {
				l[*label.Key] = *label.Value
			}
		}
	}

	return l, nil
}

// ImageName returns a full image name (OCI container tag) for the
// Function based off of the Function's `Registry` member plus `Name`.
// Used to calculate the final value for .Deploy.Image when none is provided
// explicitly.
//
// form:    [registry]/[user]/[function]:latest
// example: quay.io/alice/my.function.name:latest
//
// Also nested namespaces should be supported:
// form:    [registry]/[parent]/[user]/[function]:latest
// example: quay.io/project/alice/my.function.name:latest
//
// Registry values which only contain a single token are presumed to
// indicate the namespace at the default registry.
func (f Function) ImageName() (image string, err error) {
	if f.Registry == "" {
		return "", ErrRegistryRequired
	}
	if f.Name == "" {
		return "", ErrNameRequired
	}

	f.Registry = strings.Trim(f.Registry, "/") // too defensive?

	// Explicitly append :latest tag.  We expect source control to drive
	// versioning, rather than rely on image tags with explicitly pinned version
	// numbers, as is seen in many serverless solutions.  This will be updated
	// to branch name when we add source-driven canary/ bluegreen deployments.
	// For pinning to an exact container image, see ImageWithDigest
	refStr := f.Registry + "/" + f.Name + ":latest"

	ref, err := name.ParseReference(refStr)
	if err != nil {
		return "", fmt.Errorf("cannot determine function image: %w", err)
	}

	return ref.Name(), nil
}

// Format yaml unmarshall error to be more human friendly.
func formatUnmarshalError(err error) error {
	var (
		e      = err.Error()
		rxp    = regexp.MustCompile("not found in type .*")
		header = fmt.Sprintf("'%v' is not valid:\n", FunctionFile)
	)

	if strings.HasPrefix(e, "yaml: unmarshal errors:") {
		e = rxp.ReplaceAllString(e, "is not valid")
		e = strings.Replace(e, "yaml: unmarshal errors:\n", header, 1)
	} else if strings.HasPrefix(e, "yaml:") {
		e = rxp.ReplaceAllString(e, "is not valid")
		e = strings.Replace(e, "yaml: ", header+"  ", 1)
	}

	return errors.New(e)
}

// Regex used during instantiation and validation of various function fields
// by labels, envs, options, etc.
var (
	regWholeSecret      = regexp.MustCompile(`^{{\s*secret:((?:\w|['-]\w)+)\s*}}$`)
	regKeyFromSecret    = regexp.MustCompile(`^{{\s*secret:((?:\w|['-]\w)+):([-._a-zA-Z0-9]+)\s*}}$`)
	regWholeConfigMap   = regexp.MustCompile(`^{{\s*configMap:((?:\w|['-]\w)+)\s*}}$`)
	regKeyFromConfigMap = regexp.MustCompile(`^{{\s*configMap:((?:\w|['-]\w)+):([-._a-zA-Z0-9]+)\s*}}$`)
	regLocalEnv         = regexp.MustCompile(`^{{\s*env:(\w+)\s*}}$`)
)

// Built returns true if the function is considered built.
// Note that this only considers the function as it exists on-disk at
// f.Root.
func (f Function) Built() bool {
	// If there is no build stamp, it is not built.
	stamp := f.BuildStamp()
	if stamp == "" {
		return false
	}

	// Calculate the current filesystem hash and see if it has changed.
	//
	// If this comparison returns true, the Function has a populated image,
	// existing buildstamp, and the calculated fingerprint has not changed.
	//
	// It's a pretty good chance the thing doesn't need to be rebuilt, though
	// of course filesystem racing conditions do exist, including both direct
	// source code modifications or changes to the image cache.
	hash, _, err := Fingerprint(f.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error calculating function's fingerprint: %v\n", err)
		return false
	}

	if stamp != hash {
		return false
	}

	// Special case of registry change on a subsequent deploy attempt should
	// result in unbuilt image, forcing a rebuild if possible
	// Example: Deploy with image using registry X. Then subsequently deploy with
	// --registry=Y, changing registry resulting in unmatched Registry and Build.Image.

	// If f.Image is specified, registry is overridden -- meaning its not taken into
	// consideration and can be different from actually built image.
	buildImage := f.Build.Image
	fRegistry := f.Registry
	if !strings.Contains(buildImage, fRegistry) && f.Image == "" {
		fmt.Fprintf(os.Stderr, "Warning: registry '%s' does not match currently built image '%s' and no direct image override was provided via --image\n", f.Registry, f.Build.Image)
		return false
	}
	return true
}

// BuildStamp accesses the current (last) build stamp for the function.
// Unbuilt functions return empty string.
func (f Function) BuildStamp() string {
	path := filepath.Join(f.Root, RunDataDir, BuiltHash)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// localSettings returns the local settings set for the function
func (f Function) newLocal() (localConfig Local, err error) {
	err = ensureRunDataDir(f.Root)
	if err != nil {
		return
	}
	localSettingsPath := filepath.Join(f.Root, RunDataDir, RunDataLocalFile)
	if _, err = os.Stat(localSettingsPath); os.IsNotExist(err) {
		err = nil
		return
	}
	b, err := os.ReadFile(localSettingsPath)
	if err != nil {
		return
	}

	err = yaml.Unmarshal(b, &localConfig)
	return
}

// WriteRuntimeBuiltImage writes built image name into runtime metadata
// directory (.func/) from f.Build.Image
func (f Function) WriteRuntimeBuiltImage(verbose bool) error {
	path := filepath.Join(f.Root, RunDataDir, BuiltImage)

	// dont write if empty (not built)
	if f.Build.Image == "" {
		return nil
	}

	if verbose {
		fmt.Printf("Writing built image: '%s' at path: '%s'\n", f.Build.Image, path)
	}

	return os.WriteFile(path, []byte(f.Build.Image), os.ModePerm)
}

// getLastBuiltImage reads .func/built-image and returns its value or empty string
// if the file doesnt exist (not built yet). Other errors are returned as usual.
func (f Function) getLastBuiltImage() (string, error) {
	path := filepath.Join(f.Root, RunDataDir, BuiltImage)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ImageNameWithDigest works with f.Build.Image and image digest. if the func
// parameter newDigest is empty, just return the image name as is.
// TODO: This function is a temporary one for a workaround for a current
// solution on how the image digest is fetched (which is during/after Push).
// Image digest should be gotten from imageID right after building the Function.
// PS: I think that imageID item contains "sha256:[digest]"
func (f Function) ImageNameWithDigest(newDigest string) string {
	if newDigest == "" {
		return f.Build.Image
	}
	image := f.Build.Image

	// overwrite current digest
	shaIndex := strings.Index(image, "@sha256:")
	if shaIndex > 0 {
		return image[:shaIndex] + "@" + newDigest
	}

	// image doesnt have a digest yet == image not pushed yet
	//parse f.Build.Image to separate its name and tag
	lastSlashIdx := strings.LastIndexAny(image, "/")
	imageAsBytes := []byte(image)
	part1 := string(imageAsBytes[:lastSlashIdx+1])
	part2 := string(imageAsBytes[lastSlashIdx+1:])
	// Remove tag from the image name and append SHA256 hash instead
	return part1 + strings.Split(part2, ":")[0] + "@" + newDigest
}
