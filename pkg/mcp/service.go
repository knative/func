package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"knative.dev/func/pkg/builders"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/knative"
	"knative.dev/func/pkg/utils"
)

// Service implements MCP tool operations using the functions client API.
type Service struct {
	factory ClientFactory
}

func NewService(factory ClientFactory) *Service {
	return &Service{factory: factory}
}

func (s *Service) client(cfg ClientConfig, options ...fn.Option) (*fn.Client, func()) {
	return s.factory(cfg, options...)
}

func loadFunction(path string) (fn.Function, error) {
	f, err := fn.NewFunction(path)
	if err != nil {
		return f, err
	}
	if !f.Initialized() {
		return f, fn.NewErrNotInitialized(f.Root)
	}
	return f, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefBool(p *bool) bool {
	return p != nil && *p
}

func derefBoolDefault(p *bool, defaultVal bool) bool {
	if p == nil {
		return defaultVal
	}
	return *p
}

// Create initializes a new function project.
func (s *Service) Create(ctx context.Context, input CreateInput) (CreateOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	opts := []fn.Option{}
	if input.Repository != nil && *input.Repository != "" {
		opts = append(opts, fn.WithRepository(*input.Repository))
	}
	client, done := s.client(cfg, opts...)
	defer done()

	template := fn.DefaultTemplate
	if input.Template != nil {
		template = *input.Template
	}

	f, err := client.Init(fn.Function{
		Root:     input.Path,
		Runtime:  input.Language,
		Template: template,
	})
	if err != nil {
		return CreateOutput{}, err
	}

	msg := fmt.Sprintf("Created %s function at %s", input.Language, f.Root)
	return CreateOutput{
		Runtime:  input.Language,
		Template: input.Template,
		Message:  msg,
	}, nil
}

// Build builds a function's container image.
func (s *Service) Build(ctx context.Context, input BuildInput) (BuildOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return BuildOutput{}, err
	}

	builder := derefString(input.Builder)
	if builder == "" {
		builder = f.Build.Builder
	}
	if builder == "" {
		builder = builders.Pack
	}

	cfg := ClientConfig{
		Verbose:            derefBool(input.Verbose),
		InsecureSkipVerify: derefBool(input.RegistryInsecure),
	}
	clientOpts, err := builderClientOptions(builder, cfg, derefBool(input.BuildTimestamp), "")
	if err != nil {
		return BuildOutput{}, err
	}
	client, done := s.client(cfg, clientOpts...)
	defer done()

	f = applyBuildInput(f, input)

	buildOpts, err := platformBuildOptions(derefString(input.Platform))
	if err != nil {
		return BuildOutput{}, err
	}

	if err = client.Scaffold(ctx, f, ""); err != nil {
		return BuildOutput{}, err
	}
	if f, err = client.Build(ctx, f, buildOpts...); err != nil {
		return BuildOutput{}, err
	}
	if derefBool(input.Push) {
		if f, _, err = client.Push(ctx, f); err != nil {
			return BuildOutput{}, err
		}
	}
	if err = f.Write(); err != nil {
		return BuildOutput{}, err
	}

	return BuildOutput{
		Image:   f.Build.Image,
		Message: fmt.Sprintf("Function built: %s", f.Build.Image),
	}, nil
}

func applyBuildInput(f fn.Function, input BuildInput) fn.Function {
	if input.Registry != nil {
		f.Registry = *input.Registry
	}
	if input.Image != nil {
		f.Image = *input.Image
	}
	if input.Builder != nil {
		f.Build.Builder = *input.Builder
	}
	if input.BuilderImage != nil && f.Build.Builder != "" {
		if f.Build.BuilderImages == nil {
			f.Build.BuilderImages = map[string]string{}
		}
		f.Build.BuilderImages[f.Build.Builder] = *input.BuilderImage
	}
	if input.RegistryInsecure != nil {
		f.RegistryInsecure = *input.RegistryInsecure
	}
	return f
}

// Deploy builds (if needed) and deploys a function.
func (s *Service) Deploy(ctx context.Context, input DeployInput) (DeployOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return DeployOutput{}, err
	}

	builder := derefString(input.Builder)
	if builder == "" {
		builder = f.Build.Builder
	}
	if builder == "" {
		builder = builders.Pack
	}

	f = applyDeployInput(f, input)

	cfg := ClientConfig{
		Verbose:            derefBool(input.Verbose),
		InsecureSkipVerify: derefBool(input.RegistryInsecure),
	}
	clientOpts, err := deployClientOptions(builder, f.Deploy.Deployer, cfg, derefBool(input.BuildTimestamp))
	if err != nil {
		return DeployOutput{}, err
	}
	client, done := s.client(cfg, clientOpts...)
	defer done()

	if derefBool(input.Remote) {
		if err = f.Write(); err != nil {
			return DeployOutput{}, err
		}
		url, f, err := client.RunPipeline(ctx, f)
		if err != nil {
			return DeployOutput{}, err
		}
		if err = f.Write(); err != nil {
			return DeployOutput{}, err
		}
		return DeployOutput{
			URL:     url,
			Image:   f.Deploy.Image,
			Message: fmt.Sprintf("Function deployed at %s", url),
		}, nil
	}

	buildFlag := derefString(input.Build)
	if buildFlag == "" {
		buildFlag = "auto"
	}

	var buildOpts []fn.BuildOption
	if buildOpts, err = platformBuildOptions(derefString(input.Platform)); err != nil {
		return DeployOutput{}, err
	}

	digested := false
	if input.Image != nil && *input.Image != "" {
		digested, err = isDigestedImage(*input.Image)
		if err != nil {
			return DeployOutput{}, err
		}
		if !digested {
			f.Deploy.Image = *input.Image
		}
	}

	if digested {
		f.Deploy.Image = *input.Image
	} else {
		doBuild, err := shouldBuild(buildFlag, f)
		if err != nil {
			return DeployOutput{}, err
		}
		if doBuild {
			if err = client.Scaffold(ctx, f, ""); err != nil {
				return DeployOutput{}, err
			}
			if f, err = client.Build(ctx, f, buildOpts...); err != nil {
				return DeployOutput{}, err
			}
		}
		push := derefBoolDefault(input.Push, true)
		if push {
			var pushed bool
			if f, pushed, err = client.Push(ctx, f); err != nil {
				return DeployOutput{}, err
			}
			if (doBuild || pushed) && f.Build.Image != "" {
				f.Deploy.Image = f.Build.Image
			}
		} else if doBuild && f.Build.Image != "" {
			f.Deploy.Image = f.Build.Image
		}
	}

	skipBuiltCheck := buildFlag == "false"
	if f, err = client.Deploy(ctx, f, fn.WithDeploySkipBuildCheck(skipBuiltCheck)); err != nil {
		return DeployOutput{}, err
	}
	if err = f.Write(); err != nil {
		return DeployOutput{}, err
	}

	instance, err := client.Describe(ctx, "", "", f)
	if err != nil {
		return DeployOutput{
			Image:   f.Deploy.Image,
			Message: fmt.Sprintf("Function deployed (describe unavailable: %v)", err),
		}, nil
	}

	return DeployOutput{
		URL:     instance.Route,
		Image:   f.Deploy.Image,
		Message: fmt.Sprintf("Function deployed at %s", instance.Route),
	}, nil
}

func applyDeployInput(f fn.Function, input DeployInput) fn.Function {
	f = applyBuildInput(f, BuildInput{
		Path:             input.Path,
		Builder:          input.Builder,
		Registry:         input.Registry,
		BuilderImage:     input.BuilderImage,
		Image:            input.Image,
		Platform:         input.Platform,
		RegistryInsecure: input.RegistryInsecure,
		BuildTimestamp:   input.BuildTimestamp,
	})
	if input.Namespace != nil {
		f.Namespace = *input.Namespace
	}
	if input.GitURL != nil {
		f.Build.Git.URL = *input.GitURL
	}
	if input.GitBranch != nil {
		f.Build.Git.Revision = *input.GitBranch
	}
	if input.GitDir != nil {
		f.Build.Git.ContextDir = *input.GitDir
	}
	if input.Domain != nil {
		f.Domain = *input.Domain
	}
	if input.PVCSize != nil {
		f.Build.PVCSize = *input.PVCSize
	}
	if input.ServiceAccount != nil {
		f.Deploy.ServiceAccountName = *input.ServiceAccount
	}
	if input.RemoteStorageClass != nil {
		f.Build.RemoteStorageClass = *input.RemoteStorageClass
	}
	if derefBool(input.Remote) {
		f.Local.Remote = true
	}
	return f
}

// List returns deployed functions.
func (s *Service) List(ctx context.Context, input ListInput) (ListOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	client, done := s.client(cfg)
	defer done()

	namespace := ""
	if input.AllNamespaces == nil || !*input.AllNamespaces {
		namespace = derefString(input.Namespace)
	}

	items, err := client.List(ctx, namespace)
	if err != nil {
		return ListOutput{}, err
	}

	output := derefString(input.Output)
	if output == "" {
		output = "human"
	}

	switch output {
	case "json":
		b, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return ListOutput{}, err
		}
		return ListOutput{Message: string(b), Functions: items}, nil
	default:
		if len(items) == 0 {
			return ListOutput{Message: "No functions found", Functions: items}, nil
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", item.Name, item.Namespace, item.Runtime, item.URL)
		}
		return ListOutput{Message: strings.TrimRight(b.String(), "\n"), Functions: items}, nil
	}
}

// Delete removes a deployed function.
func (s *Service) Delete(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	client, done := s.client(cfg)
	defer done()

	all := true
	if input.All != nil {
		all = *input.All
	}

	var name, namespace string
	if input.Name != nil {
		name = *input.Name
		namespace = derefString(input.Namespace)
	} else if input.Path != nil {
		f, err := loadFunction(*input.Path)
		if err != nil {
			return DeleteOutput{}, err
		}
		name = f.Name
		namespace = f.Deploy.Namespace
		if input.Namespace != nil {
			namespace = *input.Namespace
		}
	}

	if err := client.Remove(ctx, name, namespace, fn.Function{}, all); err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{Message: fmt.Sprintf("Function %q deleted", name)}, nil
}

// Describe returns details about a deployed function.
func (s *Service) Describe(ctx context.Context, input DescribeInput) (DescribeOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	client, done := s.client(cfg)
	defer done()

	var (
		instance fn.Instance
		err      error
	)
	if input.Name != nil && *input.Name != "" {
		instance, err = client.Describe(ctx, *input.Name, derefString(input.Namespace), fn.Function{})
	} else {
		var f fn.Function
		f, err = loadFunction(input.Path)
		if err != nil {
			return DescribeOutput{}, err
		}
		instance, err = client.Describe(ctx, "", "", f)
	}
	if err != nil {
		return DescribeOutput{}, err
	}

	output := derefString(input.Output)
	if output == "json" {
		b, err := json.MarshalIndent(instance, "", "  ")
		if err != nil {
			return DescribeOutput{}, err
		}
		return DescribeOutput{Instance: instance, Message: string(b)}, nil
	}

	msg := fmt.Sprintf("Name: %s\nNamespace: %s\nRoute: %s\nImage: %s\nDeployer: %s\nReady: %s",
		instance.Name, instance.Namespace, instance.Route, instance.Image, instance.Deployer, instance.Ready)
	return DescribeOutput{Instance: instance, Message: msg}, nil
}

// Invoke sends a test request to a running function.
func (s *Service) Invoke(ctx context.Context, input InvokeInput) (InvokeOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	client, done := s.client(cfg)
	defer done()

	msg := fn.NewInvokeMessage()
	if input.Data != nil {
		msg.Data = []byte(*input.Data)
	}
	if input.ContentType != nil {
		msg.ContentType = *input.ContentType
	}
	if input.Source != nil {
		msg.Source = *input.Source
	}
	if input.Type != nil {
		msg.Type = *input.Type
	}
	if input.Format != nil {
		msg.Format = *input.Format
	}
	if input.RequestType != nil {
		msg.RequestType = *input.RequestType
	}

	target := derefString(input.Target)
	metadata, body, err := client.Invoke(ctx, input.Path, target, msg)
	if err != nil {
		return InvokeOutput{}, err
	}
	return InvokeOutput{
		Body:     body,
		Metadata: metadata,
		Message:  "Invocation succeeded",
	}, nil
}

// Run starts a function locally.
func (s *Service) Run(ctx context.Context, input RunInput) (RunOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return RunOutput{}, err
	}

	builder := f.Build.Builder
	if builder == "" {
		builder = builders.Pack
	}

	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	opts, err := builderClientOptions(builder, cfg, false, "")
	if err != nil {
		return RunOutput{}, err
	}

	container := builder != builders.Host
	if container {
		opts = append(opts, fn.WithRunner(docker.NewRunner(cfg.Verbose, os.Stdout, os.Stderr)))
	}

	client, done := s.client(cfg, opts...)
	defer done()

	if container {
		if err = client.Scaffold(ctx, f, ""); err != nil {
			return RunOutput{}, err
		}
		if f, err = client.Build(ctx, f); err != nil {
			return RunOutput{}, err
		}
	}

	var runOpts []fn.RunOption
	if input.Address != nil {
		runOpts = append(runOpts, fn.RunWithAddress(*input.Address))
	}

	job, err := client.Run(ctx, f, runOpts...)
	if err != nil {
		return RunOutput{}, err
	}

	addr := net.JoinHostPort(job.Host, job.Port)
	url := fmt.Sprintf("http://%s", addr)
	return RunOutput{
		URL:     url,
		Address: addr,
		Message: fmt.Sprintf("Function running at %s", url),
	}, nil
}

// Logs streams logs from a deployed function (returns collected output for MCP).
func (s *Service) Logs(ctx context.Context, input LogsInput) (LogsOutput, error) {
	cfg := ClientConfig{Verbose: derefBool(input.Verbose)}
	client, done := s.client(cfg)
	defer done()

	var f fn.Function
	var deployer string
	if input.Name != nil && *input.Name != "" {
		instance, err := client.Describe(ctx, *input.Name, derefString(input.Namespace), fn.Function{})
		if err != nil {
			return LogsOutput{}, fmt.Errorf("failed to get function details: %w", err)
		}
		f.Name = instance.Name
		f.Namespace = instance.Namespace
		f.Image = instance.Image
		deployer = instance.Deployer
	} else {
		var err error
		f, err = loadFunction(input.Path)
		if err != nil {
			return LogsOutput{}, err
		}
		instance, err := client.Describe(ctx, "", "", f)
		if err != nil {
			return LogsOutput{}, fmt.Errorf("function not deployed or not found: %w", err)
		}
		f.Name = instance.Name
		f.Namespace = instance.Namespace
		f.Image = instance.Image
		deployer = instance.Deployer
	}

	if deployer != "" && deployer != "knative" {
		return LogsOutput{}, fmt.Errorf("'func logs' is not yet supported for the %q deployer", deployer)
	}

	since := derefString(input.Since)
	if since == "" {
		since = "1m"
	}
	duration, err := time.ParseDuration(since)
	if err != nil {
		return LogsOutput{}, fmt.Errorf("invalid --since duration: %w", err)
	}
	sinceTime := time.Now().Add(-duration)

	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var buf strings.Builder
	err = knative.GetKServiceLogs(logCtx, f.Namespace, f.Name, f.Image, &sinceTime, &buf)
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		return LogsOutput{}, fmt.Errorf("failed to stream logs: %w", err)
	}

	return LogsOutput{
		Message: buf.String(),
	}, nil
}

// --- Config operations ---

func (s *Service) ConfigEnvsList(_ context.Context, input ConfigEnvsListInput) (ConfigEnvsListOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigEnvsListOutput{}, err
	}
	if len(f.Run.Envs) == 0 {
		return ConfigEnvsListOutput{Message: "There aren't any configured environment variables"}, nil
	}
	var b strings.Builder
	for _, e := range f.Run.Envs {
		fmt.Fprintf(&b, " - %s\n", e.String())
	}
	return ConfigEnvsListOutput{Message: strings.TrimRight(b.String(), "\n")}, nil
}

func (s *Service) ConfigEnvsAdd(_ context.Context, input ConfigEnvsAddInput) (ConfigEnvsAddOutput, error) {
	if err := input.validate(); err != nil {
		return ConfigEnvsAddOutput{}, err
	}
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigEnvsAddOutput{}, err
	}

	var namePtr, valuePtr *string
	if input.Name != nil {
		if err := utils.ValidateEnvVarName(*input.Name); err != nil {
			return ConfigEnvsAddOutput{}, err
		}
		namePtr = input.Name
	}

	if input.Value != nil {
		valuePtr = input.Value
	} else if input.SecretName != nil {
		var v string
		if input.SecretKey != nil {
			v = fmt.Sprintf("{{ secret:%s:%s }}", *input.SecretName, *input.SecretKey)
		} else {
			v = fmt.Sprintf("{{ secret:%s }}", *input.SecretName)
		}
		valuePtr = &v
	} else if input.ConfigMapName != nil {
		var v string
		if input.ConfigMapKey != nil {
			v = fmt.Sprintf("{{ configMap:%s:%s }}", *input.ConfigMapName, *input.ConfigMapKey)
		} else {
			v = fmt.Sprintf("{{ configMap:%s }}", *input.ConfigMapName)
		}
		valuePtr = &v
	}

	f.Run.Envs = append(f.Run.Envs, fn.Env{Name: namePtr, Value: valuePtr})
	if err = f.Write(); err != nil {
		return ConfigEnvsAddOutput{}, err
	}
	return ConfigEnvsAddOutput{Message: "Environment variable added"}, nil
}

func (s *Service) ConfigEnvsRemove(_ context.Context, input ConfigEnvsRemoveInput) (ConfigEnvsRemoveOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigEnvsRemoveOutput{}, err
	}
	if input.Name == nil {
		return ConfigEnvsRemoveOutput{}, fmt.Errorf("name is required")
	}
	envs := []fn.Env{}
	for _, e := range f.Run.Envs {
		if e.Name == nil || *e.Name != *input.Name {
			envs = append(envs, e)
		}
	}
	f.Run.Envs = envs
	if err = f.Write(); err != nil {
		return ConfigEnvsRemoveOutput{}, err
	}
	return ConfigEnvsRemoveOutput{Message: fmt.Sprintf("Environment variable %q removed", *input.Name)}, nil
}

func (s *Service) ConfigLabelsList(_ context.Context, input ConfigLabelsListInput) (ConfigLabelsListOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigLabelsListOutput{}, err
	}
	if len(f.Deploy.Labels) == 0 {
		return ConfigLabelsListOutput{Message: "There aren't any configured labels"}, nil
	}
	var b strings.Builder
	for _, l := range f.Deploy.Labels {
		fmt.Fprintf(&b, " - %s\n", l.String())
	}
	return ConfigLabelsListOutput{Message: strings.TrimRight(b.String(), "\n")}, nil
}

func (s *Service) ConfigLabelsAdd(_ context.Context, input ConfigLabelsAddInput) (ConfigLabelsAddOutput, error) {
	if input.Name == nil || input.Value == nil {
		return ConfigLabelsAddOutput{}, fmt.Errorf("name and value are required")
	}
	if err := utils.ValidateLabelKey(*input.Name); err != nil {
		return ConfigLabelsAddOutput{}, err
	}
	if err := utils.ValidateLabelValue(*input.Value); err != nil {
		return ConfigLabelsAddOutput{}, err
	}
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigLabelsAddOutput{}, err
	}
	f.Deploy.Labels = append(f.Deploy.Labels, fn.Label{Key: input.Name, Value: input.Value})
	if err = f.Write(); err != nil {
		return ConfigLabelsAddOutput{}, err
	}
	return ConfigLabelsAddOutput{Message: "Label added"}, nil
}

func (s *Service) ConfigLabelsRemove(_ context.Context, input ConfigLabelsRemoveInput) (ConfigLabelsRemoveOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigLabelsRemoveOutput{}, err
	}
	if input.Name == nil {
		return ConfigLabelsRemoveOutput{}, fmt.Errorf("name is required")
	}
	labels := []fn.Label{}
	for _, l := range f.Deploy.Labels {
		if l.Key == nil || *l.Key != *input.Name {
			labels = append(labels, l)
		}
	}
	f.Deploy.Labels = labels
	if err = f.Write(); err != nil {
		return ConfigLabelsRemoveOutput{}, err
	}
	return ConfigLabelsRemoveOutput{Message: fmt.Sprintf("Label %q removed", *input.Name)}, nil
}

func (s *Service) ConfigVolumesList(_ context.Context, input ConfigVolumesListInput) (ConfigVolumesListOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigVolumesListOutput{}, err
	}
	if len(f.Run.Volumes) == 0 {
		return ConfigVolumesListOutput{Message: "There aren't any configured volume mounts"}, nil
	}
	var b strings.Builder
	for _, v := range f.Run.Volumes {
		fmt.Fprintf(&b, " - %s\n", v.String())
	}
	return ConfigVolumesListOutput{Message: strings.TrimRight(b.String(), "\n")}, nil
}

func (s *Service) ConfigVolumesAdd(_ context.Context, input ConfigVolumesAddInput) (ConfigVolumesAddOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigVolumesAddOutput{}, err
	}
	vol, err := volumeFromInput(input)
	if err != nil {
		return ConfigVolumesAddOutput{}, err
	}
	f.Run.Volumes = append(f.Run.Volumes, vol)
	if err = f.Write(); err != nil {
		return ConfigVolumesAddOutput{}, err
	}
	return ConfigVolumesAddOutput{Message: "Volume added"}, nil
}

func volumeFromInput(input ConfigVolumesAddInput) (fn.Volume, error) {
	if input.MountPath == nil || *input.MountPath == "" {
		return fn.Volume{}, fmt.Errorf("mountPath is required")
	}
	if !strings.HasPrefix(*input.MountPath, "/") {
		return fn.Volume{}, fmt.Errorf("mount path must be an absolute path (start with /)")
	}
	volType := derefString(input.Type)
	if volType == "" {
		return fn.Volume{}, fmt.Errorf("type is required")
	}

	mountPath := *input.MountPath
	newVolume := fn.Volume{Path: &mountPath}
	source := derefString(input.Source)

	switch volType {
	case "configmap":
		if source == "" {
			return fn.Volume{}, fmt.Errorf("source is required for configmap volumes")
		}
		newVolume.ConfigMap = &source
	case "secret":
		if source == "" {
			return fn.Volume{}, fmt.Errorf("source is required for secret volumes")
		}
		newVolume.Secret = &source
	case "pvc":
		if source == "" {
			return fn.Volume{}, fmt.Errorf("source is required for pvc volumes")
		}
		newVolume.PersistentVolumeClaim = &fn.PersistentVolumeClaim{
			ClaimName: &source,
			ReadOnly:  derefBool(input.ReadOnly),
		}
	case "emptydir":
		emptyDir := &fn.EmptyDir{}
		if input.Size != nil {
			emptyDir.SizeLimit = input.Size
		}
		if input.Medium != nil {
			medium := *input.Medium
			if medium != fn.StorageMediumMemory && medium != fn.StorageMediumDefault && medium != "" {
				return fn.Volume{}, fmt.Errorf("invalid medium: must be 'Memory' or empty")
			}
			emptyDir.Medium = medium
		}
		newVolume.EmptyDir = emptyDir
	default:
		return fn.Volume{}, fmt.Errorf("invalid volume type: %s", volType)
	}
	return newVolume, nil
}

func (s *Service) ConfigVolumesRemove(_ context.Context, input ConfigVolumesRemoveInput) (ConfigVolumesRemoveOutput, error) {
	f, err := loadFunction(input.Path)
	if err != nil {
		return ConfigVolumesRemoveOutput{}, err
	}
	if input.MountPath == nil {
		return ConfigVolumesRemoveOutput{}, fmt.Errorf("mountPath is required")
	}
	volumes := []fn.Volume{}
	for _, v := range f.Run.Volumes {
		if v.Path == nil || *v.Path != *input.MountPath {
			volumes = append(volumes, v)
		}
	}
	f.Run.Volumes = volumes
	if err = f.Write(); err != nil {
		return ConfigVolumesRemoveOutput{}, err
	}
	return ConfigVolumesRemoveOutput{Message: fmt.Sprintf("Volume at %q removed", *input.MountPath)}, nil
}

// Runtimes returns available language runtimes.
func (s *Service) Runtimes() (string, error) {
	cfg := ClientConfig{}
	client, done := s.client(cfg)
	defer done()
	runtimes, err := client.Runtimes()
	if err != nil {
		return "", err
	}
	return strings.Join(runtimes, "\n"), nil
}

// FunctionState returns JSON representation of a function at path.
func (s *Service) FunctionState(path string) ([]byte, error) {
	f, err := loadFunction(path)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(f, "", "  ")
}
