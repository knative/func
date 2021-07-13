package buildpacks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/logging"

	dockerClient "github.com/docker/docker/client"

	fn "github.com/boson-project/func"
)

//Builder holds the configuration that will be passed to
//Buildpack builder
type Builder struct {
	Verbose bool
}

//NewBuilder builds the new Builder configuration
func NewBuilder() *Builder {
	return &Builder{}
}

//RuntimeToBuildpack holds the mapping between the Runtime and its corresponding
//Buildpack builder to use
var RuntimeToBuildpack = map[string]string{
	"quarkus":    "quay.io/boson/faas-quarkus-builder",
	"node":       "quay.io/boson/faas-nodejs-builder",
	"go":         "quay.io/boson/faas-go-builder",
	"springboot": "quay.io/boson/faas-springboot-builder",
	"python":     "quay.io/boson/faas-python-builder",
	"typescript": "quay.io/boson/faas-nodejs-builder",
	"rust":       "quay.io/boson/faas-rust-builder",
}

// Build the Function at path.
func (builder *Builder) Build(ctx context.Context, f fn.Function) (err error) {

	// Use the builder found in the Function configuration file
	// If one isn't found, use the defaults
	var packBuilder string
	if f.Builder != "" {
		packBuilder = f.Builder
		pb, ok := f.BuilderMap[packBuilder]
		if ok {
			packBuilder = pb
		}
	} else {
		packBuilder = RuntimeToBuildpack[f.Runtime]
		if packBuilder == "" {
			return errors.New(fmt.Sprint("unsupported runtime: ", f.Runtime))
		}
	}

	// Build options for the pack client.
	var network string
	if runtime.GOOS == "linux" {
		network = "host"
	}

	// log output is either STDOUt or kept in a buffer to be printed on error.
	var logWriter io.Writer
	if builder.Verbose {
		// pass stdout as non-closeable writer
		// otherwise pack client would close it which is bad
		logWriter = stdoutWrapper{os.Stdout}
	} else {
		logWriter = &bytes.Buffer{}
	}

	// Client with a logger which is enabled if in Verbose mode.
	dockerClient, err := dockerClient.NewClientWithOpts(
		dockerClient.FromEnv,
		dockerClient.WithVersion("1.38"),
	)
	if err != nil {
		return err
	}

	version, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		return err
	}

	var deamonIsPodman bool
	for _, component := range version.Components {
		if component.Name == "Podman Engine" {
			deamonIsPodman = true
			break
		}
	}

	packOpts := pack.BuildOptions{
		AppPath:      f.Root,
		Image:        f.Image,
		Builder:      packBuilder,
		TrustBuilder: !deamonIsPodman && strings.HasPrefix(packBuilder, "quay.io/boson"),
		DockerHost:   os.Getenv("DOCKER_HOST"),
		ContainerConfig: struct {
			Network string
			Volumes []string
		}{Network: network, Volumes: nil},
	}

	dockerClientWrapper := &clientWrapper{dockerClient}
	packClient, err := pack.NewClient(pack.WithLogger(logging.New(logWriter)), pack.WithDockerClient(dockerClientWrapper))
	if err != nil {
		return
	}

	// Build based using the given builder.
	if err = packClient.Build(ctx, packOpts); err != nil {
		if ctx.Err() != nil {
			// received SIGINT
			return
		} else if !builder.Verbose {
			// If the builder was not showing logs, embed the full logs in the error.
			err = fmt.Errorf("%v\noutput: %s\n", err, logWriter.(*bytes.Buffer).String())
		}
	}

	return
}

// hack this makes stdout non-closeable
type stdoutWrapper struct {
	impl io.Writer
}

func (s stdoutWrapper) Write(p []byte) (n int, err error) {
	return s.impl.Write(p)
}

type clientWrapper struct {
	impl dockerClient.CommonAPIClient
}

// The following section is a workaround until https://github.com/buildpacks/pack/issues/1208 is fixed.
// override injecting the security opt

func (c clientWrapper) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	hostConfig.SecurityOpt = []string{"label=disable"}
	return c.impl.ContainerCreate(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

// just plain forwards

func (c clientWrapper) ConfigList(ctx context.Context, options types.ConfigListOptions) ([]swarm.Config, error) {
	return c.impl.ConfigList(ctx, options)
}

func (c clientWrapper) ConfigCreate(ctx context.Context, config swarm.ConfigSpec) (types.ConfigCreateResponse, error) {
	return c.impl.ConfigCreate(ctx, config)
}

func (c clientWrapper) ConfigRemove(ctx context.Context, id string) error {
	return c.impl.ConfigRemove(ctx, id)
}

func (c clientWrapper) ConfigInspectWithRaw(ctx context.Context, name string) (swarm.Config, []byte, error) {
	return c.impl.ConfigInspectWithRaw(ctx, name)
}

func (c clientWrapper) ConfigUpdate(ctx context.Context, id string, version swarm.Version, config swarm.ConfigSpec) error {
	return c.impl.ConfigUpdate(ctx, id, version, config)
}

func (c clientWrapper) ContainerAttach(ctx context.Context, container string, options types.ContainerAttachOptions) (types.HijackedResponse, error) {
	return c.impl.ContainerAttach(ctx, container, options)
}

func (c clientWrapper) ContainerCommit(ctx context.Context, container string, options types.ContainerCommitOptions) (types.IDResponse, error) {
	return c.impl.ContainerCommit(ctx, container, options)
}

func (c clientWrapper) ContainerDiff(ctx context.Context, container string) ([]container.ContainerChangeResponseItem, error) {
	return c.impl.ContainerDiff(ctx, container)
}

func (c clientWrapper) ContainerExecAttach(ctx context.Context, execID string, config types.ExecStartCheck) (types.HijackedResponse, error) {
	return c.impl.ContainerExecAttach(ctx, execID, config)
}

func (c clientWrapper) ContainerExecCreate(ctx context.Context, container string, config types.ExecConfig) (types.IDResponse, error) {
	return c.impl.ContainerExecCreate(ctx, container, config)
}

func (c clientWrapper) ContainerExecInspect(ctx context.Context, execID string) (types.ContainerExecInspect, error) {
	return c.impl.ContainerExecInspect(ctx, execID)
}

func (c clientWrapper) ContainerExecResize(ctx context.Context, execID string, options types.ResizeOptions) error {
	return c.impl.ContainerExecResize(ctx, execID, options)
}

func (c clientWrapper) ContainerExecStart(ctx context.Context, execID string, config types.ExecStartCheck) error {
	return c.impl.ContainerExecStart(ctx, execID, config)
}

func (c clientWrapper) ContainerExport(ctx context.Context, container string) (io.ReadCloser, error) {
	return c.impl.ContainerExport(ctx, container)
}

func (c clientWrapper) ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error) {
	return c.impl.ContainerInspect(ctx, container)
}

func (c clientWrapper) ContainerInspectWithRaw(ctx context.Context, container string, getSize bool) (types.ContainerJSON, []byte, error) {
	return c.impl.ContainerInspectWithRaw(ctx, container, getSize)
}

func (c clientWrapper) ContainerKill(ctx context.Context, container, signal string) error {
	return c.impl.ContainerKill(ctx, container, signal)
}

func (c clientWrapper) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	return c.impl.ContainerList(ctx, options)
}

func (c clientWrapper) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return c.impl.ContainerLogs(ctx, container, options)
}

func (c clientWrapper) ContainerPause(ctx context.Context, container string) error {
	return c.impl.ContainerPause(ctx, container)
}

func (c clientWrapper) ContainerRemove(ctx context.Context, container string, options types.ContainerRemoveOptions) error {
	return c.impl.ContainerRemove(ctx, container, options)
}

func (c clientWrapper) ContainerRename(ctx context.Context, container, newContainerName string) error {
	return c.impl.ContainerRename(ctx, container, newContainerName)
}

func (c clientWrapper) ContainerResize(ctx context.Context, container string, options types.ResizeOptions) error {
	return c.impl.ContainerResize(ctx, container, options)
}

func (c clientWrapper) ContainerRestart(ctx context.Context, container string, timeout *time.Duration) error {
	return c.impl.ContainerRestart(ctx, container, timeout)
}

func (c clientWrapper) ContainerStatPath(ctx context.Context, container, path string) (types.ContainerPathStat, error) {
	return c.impl.ContainerStatPath(ctx, container, path)
}

func (c clientWrapper) ContainerStats(ctx context.Context, container string, stream bool) (types.ContainerStats, error) {
	return c.impl.ContainerStats(ctx, container, stream)
}

func (c clientWrapper) ContainerStatsOneShot(ctx context.Context, container string) (types.ContainerStats, error) {
	return c.impl.ContainerStatsOneShot(ctx, container)
}

func (c clientWrapper) ContainerStart(ctx context.Context, container string, options types.ContainerStartOptions) error {
	return c.impl.ContainerStart(ctx, container, options)
}

func (c clientWrapper) ContainerStop(ctx context.Context, container string, timeout *time.Duration) error {
	return c.impl.ContainerStop(ctx, container, timeout)
}

func (c clientWrapper) ContainerTop(ctx context.Context, container string, arguments []string) (container.ContainerTopOKBody, error) {
	return c.impl.ContainerTop(ctx, container, arguments)
}

func (c clientWrapper) ContainerUnpause(ctx context.Context, container string) error {
	return c.impl.ContainerUnpause(ctx, container)
}

func (c clientWrapper) ContainerUpdate(ctx context.Context, container string, updateConfig container.UpdateConfig) (container.ContainerUpdateOKBody, error) {
	return c.impl.ContainerUpdate(ctx, container, updateConfig)
}

func (c clientWrapper) ContainerWait(ctx context.Context, container string, condition container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	return c.impl.ContainerWait(ctx, container, condition)
}

func (c clientWrapper) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, types.ContainerPathStat, error) {
	return c.impl.CopyFromContainer(ctx, container, srcPath)
}

func (c clientWrapper) CopyToContainer(ctx context.Context, container, path string, content io.Reader, options types.CopyToContainerOptions) error {
	return c.impl.CopyToContainer(ctx, container, path, content, options)
}

func (c clientWrapper) ContainersPrune(ctx context.Context, pruneFilters filters.Args) (types.ContainersPruneReport, error) {
	return c.impl.ContainersPrune(ctx, pruneFilters)
}

func (c clientWrapper) DistributionInspect(ctx context.Context, image, encodedRegistryAuth string) (registry.DistributionInspect, error) {
	return c.impl.DistributionInspect(ctx, image, encodedRegistryAuth)
}

func (c clientWrapper) ImageBuild(ctx context.Context, context io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	return c.impl.ImageBuild(ctx, context, options)
}

func (c clientWrapper) BuildCachePrune(ctx context.Context, opts types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error) {
	return c.impl.BuildCachePrune(ctx, opts)
}

func (c clientWrapper) BuildCancel(ctx context.Context, id string) error {
	return c.impl.BuildCancel(ctx, id)
}

func (c clientWrapper) ImageCreate(ctx context.Context, parentReference string, options types.ImageCreateOptions) (io.ReadCloser, error) {
	return c.impl.ImageCreate(ctx, parentReference, options)
}

func (c clientWrapper) ImageHistory(ctx context.Context, image string) ([]image.HistoryResponseItem, error) {
	return c.impl.ImageHistory(ctx, image)
}

func (c clientWrapper) ImageImport(ctx context.Context, source types.ImageImportSource, ref string, options types.ImageImportOptions) (io.ReadCloser, error) {
	return c.impl.ImageImport(ctx, source, ref, options)
}

func (c clientWrapper) ImageInspectWithRaw(ctx context.Context, image string) (types.ImageInspect, []byte, error) {
	return c.impl.ImageInspectWithRaw(ctx, image)
}

func (c clientWrapper) ImageList(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error) {
	return c.impl.ImageList(ctx, options)
}

func (c clientWrapper) ImageLoad(ctx context.Context, input io.Reader, quiet bool) (types.ImageLoadResponse, error) {
	return c.impl.ImageLoad(ctx, input, quiet)
}

func (c clientWrapper) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	return c.impl.ImagePull(ctx, ref, options)
}

func (c clientWrapper) ImagePush(ctx context.Context, ref string, options types.ImagePushOptions) (io.ReadCloser, error) {
	return c.impl.ImagePush(ctx, ref, options)
}

func (c clientWrapper) ImageRemove(ctx context.Context, image string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
	return c.impl.ImageRemove(ctx, image, options)
}

func (c clientWrapper) ImageSearch(ctx context.Context, term string, options types.ImageSearchOptions) ([]registry.SearchResult, error) {
	return c.impl.ImageSearch(ctx, term, options)
}

func (c clientWrapper) ImageSave(ctx context.Context, images []string) (io.ReadCloser, error) {
	return c.impl.ImageSave(ctx, images)
}

func (c clientWrapper) ImageTag(ctx context.Context, image, ref string) error {
	return c.impl.ImageTag(ctx, image, ref)
}

func (c clientWrapper) ImagesPrune(ctx context.Context, pruneFilter filters.Args) (types.ImagesPruneReport, error) {
	return c.impl.ImagesPrune(ctx, pruneFilter)
}

func (c clientWrapper) NodeInspectWithRaw(ctx context.Context, nodeID string) (swarm.Node, []byte, error) {
	return c.impl.NodeInspectWithRaw(ctx, nodeID)
}

func (c clientWrapper) NodeList(ctx context.Context, options types.NodeListOptions) ([]swarm.Node, error) {
	return c.impl.NodeList(ctx, options)
}

func (c clientWrapper) NodeRemove(ctx context.Context, nodeID string, options types.NodeRemoveOptions) error {
	return c.impl.NodeRemove(ctx, nodeID, options)
}

func (c clientWrapper) NodeUpdate(ctx context.Context, nodeID string, version swarm.Version, node swarm.NodeSpec) error {
	return c.impl.NodeUpdate(ctx, nodeID, version, node)
}

func (c clientWrapper) NetworkConnect(ctx context.Context, network, container string, config *network.EndpointSettings) error {
	return c.impl.NetworkConnect(ctx, network, container, config)
}

func (c clientWrapper) NetworkCreate(ctx context.Context, name string, options types.NetworkCreate) (types.NetworkCreateResponse, error) {
	return c.impl.NetworkCreate(ctx, name, options)
}

func (c clientWrapper) NetworkDisconnect(ctx context.Context, network, container string, force bool) error {
	return c.impl.NetworkDisconnect(ctx, network, container, force)
}

func (c clientWrapper) NetworkInspect(ctx context.Context, network string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	return c.impl.NetworkInspect(ctx, network, options)
}

func (c clientWrapper) NetworkInspectWithRaw(ctx context.Context, network string, options types.NetworkInspectOptions) (types.NetworkResource, []byte, error) {
	return c.impl.NetworkInspectWithRaw(ctx, network, options)
}

func (c clientWrapper) NetworkList(ctx context.Context, options types.NetworkListOptions) ([]types.NetworkResource, error) {
	return c.impl.NetworkList(ctx, options)
}

func (c clientWrapper) NetworkRemove(ctx context.Context, network string) error {
	return c.impl.NetworkRemove(ctx, network)
}

func (c clientWrapper) NetworksPrune(ctx context.Context, pruneFilter filters.Args) (types.NetworksPruneReport, error) {
	return c.impl.NetworksPrune(ctx, pruneFilter)
}

func (c clientWrapper) PluginList(ctx context.Context, filter filters.Args) (types.PluginsListResponse, error) {
	return c.impl.PluginList(ctx, filter)
}

func (c clientWrapper) PluginRemove(ctx context.Context, name string, options types.PluginRemoveOptions) error {
	return c.impl.PluginRemove(ctx, name, options)
}

func (c clientWrapper) PluginEnable(ctx context.Context, name string, options types.PluginEnableOptions) error {
	return c.impl.PluginEnable(ctx, name, options)
}

func (c clientWrapper) PluginDisable(ctx context.Context, name string, options types.PluginDisableOptions) error {
	return c.impl.PluginDisable(ctx, name, options)
}

func (c clientWrapper) PluginInstall(ctx context.Context, name string, options types.PluginInstallOptions) (io.ReadCloser, error) {
	return c.impl.PluginInstall(ctx, name, options)
}

func (c clientWrapper) PluginUpgrade(ctx context.Context, name string, options types.PluginInstallOptions) (io.ReadCloser, error) {
	return c.impl.PluginUpgrade(ctx, name, options)
}

func (c clientWrapper) PluginPush(ctx context.Context, name string, registryAuth string) (io.ReadCloser, error) {
	return c.impl.PluginPush(ctx, name, registryAuth)
}

func (c clientWrapper) PluginSet(ctx context.Context, name string, args []string) error {
	return c.impl.PluginSet(ctx, name, args)
}

func (c clientWrapper) PluginInspectWithRaw(ctx context.Context, name string) (*types.Plugin, []byte, error) {
	return c.impl.PluginInspectWithRaw(ctx, name)
}

func (c clientWrapper) PluginCreate(ctx context.Context, createContext io.Reader, options types.PluginCreateOptions) error {
	return c.impl.PluginCreate(ctx, createContext, options)
}

func (c clientWrapper) ServiceCreate(ctx context.Context, service swarm.ServiceSpec, options types.ServiceCreateOptions) (types.ServiceCreateResponse, error) {
	return c.impl.ServiceCreate(ctx, service, options)
}

func (c clientWrapper) ServiceInspectWithRaw(ctx context.Context, serviceID string, options types.ServiceInspectOptions) (swarm.Service, []byte, error) {
	return c.impl.ServiceInspectWithRaw(ctx, serviceID, options)
}

func (c clientWrapper) ServiceList(ctx context.Context, options types.ServiceListOptions) ([]swarm.Service, error) {
	return c.impl.ServiceList(ctx, options)
}

func (c clientWrapper) ServiceRemove(ctx context.Context, serviceID string) error {
	return c.impl.ServiceRemove(ctx, serviceID)
}

func (c clientWrapper) ServiceUpdate(ctx context.Context, serviceID string, version swarm.Version, service swarm.ServiceSpec, options types.ServiceUpdateOptions) (types.ServiceUpdateResponse, error) {
	return c.impl.ServiceUpdate(ctx, serviceID, version, service, options)
}

func (c clientWrapper) ServiceLogs(ctx context.Context, serviceID string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return c.impl.ServiceLogs(ctx, serviceID, options)
}

func (c clientWrapper) TaskLogs(ctx context.Context, taskID string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return c.impl.TaskLogs(ctx, taskID, options)
}

func (c clientWrapper) TaskInspectWithRaw(ctx context.Context, taskID string) (swarm.Task, []byte, error) {
	return c.impl.TaskInspectWithRaw(ctx, taskID)
}

func (c clientWrapper) TaskList(ctx context.Context, options types.TaskListOptions) ([]swarm.Task, error) {
	return c.impl.TaskList(ctx, options)
}

func (c clientWrapper) SwarmInit(ctx context.Context, req swarm.InitRequest) (string, error) {
	return c.impl.SwarmInit(ctx, req)
}

func (c clientWrapper) SwarmJoin(ctx context.Context, req swarm.JoinRequest) error {
	return c.impl.SwarmJoin(ctx, req)
}

func (c clientWrapper) SwarmGetUnlockKey(ctx context.Context) (types.SwarmUnlockKeyResponse, error) {
	return c.impl.SwarmGetUnlockKey(ctx)
}

func (c clientWrapper) SwarmUnlock(ctx context.Context, req swarm.UnlockRequest) error {
	return c.impl.SwarmUnlock(ctx, req)
}

func (c clientWrapper) SwarmLeave(ctx context.Context, force bool) error {
	return c.impl.SwarmLeave(ctx, force)
}

func (c clientWrapper) SwarmInspect(ctx context.Context) (swarm.Swarm, error) {
	return c.impl.SwarmInspect(ctx)
}

func (c clientWrapper) SwarmUpdate(ctx context.Context, version swarm.Version, swarm swarm.Spec, flags swarm.UpdateFlags) error {
	return c.impl.SwarmUpdate(ctx, version, swarm, flags)
}

func (c clientWrapper) SecretList(ctx context.Context, options types.SecretListOptions) ([]swarm.Secret, error) {
	return c.impl.SecretList(ctx, options)
}

func (c clientWrapper) SecretCreate(ctx context.Context, secret swarm.SecretSpec) (types.SecretCreateResponse, error) {
	return c.impl.SecretCreate(ctx, secret)
}

func (c clientWrapper) SecretRemove(ctx context.Context, id string) error {
	return c.impl.SecretRemove(ctx, id)
}

func (c clientWrapper) SecretInspectWithRaw(ctx context.Context, name string) (swarm.Secret, []byte, error) {
	return c.impl.SecretInspectWithRaw(ctx, name)
}

func (c clientWrapper) SecretUpdate(ctx context.Context, id string, version swarm.Version, secret swarm.SecretSpec) error {
	return c.impl.SecretUpdate(ctx, id, version, secret)
}

func (c clientWrapper) Events(ctx context.Context, options types.EventsOptions) (<-chan events.Message, <-chan error) {
	return c.impl.Events(ctx, options)
}

func (c clientWrapper) Info(ctx context.Context) (types.Info, error) { return c.impl.Info(ctx) }

func (c clientWrapper) RegistryLogin(ctx context.Context, auth types.AuthConfig) (registry.AuthenticateOKBody, error) {
	return c.impl.RegistryLogin(ctx, auth)
}

func (c clientWrapper) DiskUsage(ctx context.Context) (types.DiskUsage, error) {
	return c.impl.DiskUsage(ctx)
}

func (c clientWrapper) Ping(ctx context.Context) (types.Ping, error) { return c.impl.Ping(ctx) }

func (c clientWrapper) VolumeCreate(ctx context.Context, options volume.VolumeCreateBody) (types.Volume, error) {
	return c.impl.VolumeCreate(ctx, options)
}

func (c clientWrapper) VolumeInspect(ctx context.Context, volumeID string) (types.Volume, error) {
	return c.impl.VolumeInspect(ctx, volumeID)
}

func (c clientWrapper) VolumeInspectWithRaw(ctx context.Context, volumeID string) (types.Volume, []byte, error) {
	return c.impl.VolumeInspectWithRaw(ctx, volumeID)
}

func (c clientWrapper) VolumeList(ctx context.Context, filter filters.Args) (volume.VolumeListOKBody, error) {
	return c.impl.VolumeList(ctx, filter)
}

func (c clientWrapper) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	return c.impl.VolumeRemove(ctx, volumeID, force)
}

func (c clientWrapper) VolumesPrune(ctx context.Context, pruneFilter filters.Args) (types.VolumesPruneReport, error) {
	return c.impl.VolumesPrune(ctx, pruneFilter)
}

func (c clientWrapper) ClientVersion() string { return c.impl.ClientVersion() }

func (c clientWrapper) DaemonHost() string { return c.impl.DaemonHost() }

func (c clientWrapper) HTTPClient() *http.Client { return c.impl.HTTPClient() }

func (c clientWrapper) ServerVersion(ctx context.Context) (types.Version, error) {
	return c.impl.ServerVersion(ctx)
}

func (c clientWrapper) NegotiateAPIVersion(ctx context.Context) {
	c.impl.NegotiateAPIVersion(ctx)
}

func (c clientWrapper) NegotiateAPIVersionPing(ping types.Ping) {
	c.impl.NegotiateAPIVersionPing(ping)
}

func (c clientWrapper) DialHijack(ctx context.Context, url, proto string, meta map[string][]string) (net.Conn, error) {
	return c.impl.DialHijack(ctx, url, proto, meta)
}

func (c clientWrapper) Dialer() func(context.Context) (net.Conn, error) { return c.impl.Dialer() }

func (c clientWrapper) Close() error { return c.impl.Close() }
