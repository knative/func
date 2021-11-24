package docker

import (
	"context"
	"io"
	"net"
	"net/http"
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
)

//<editor-fold desc="plain forwards to pimpl">

func (w clientWithAdditionalCleanup) BuildCachePrune(arg0 context.Context, arg1 types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error) {
	return w.pimpl.BuildCachePrune(arg0, arg1)
}

func (w clientWithAdditionalCleanup) BuildCancel(arg0 context.Context, arg1 string) error {
	return w.pimpl.BuildCancel(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ClientVersion() string {
	return w.pimpl.ClientVersion()
}

func (w clientWithAdditionalCleanup) ConfigCreate(arg0 context.Context, arg1 swarm.ConfigSpec) (types.ConfigCreateResponse, error) {
	return w.pimpl.ConfigCreate(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ConfigInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Config, []uint8, error) {
	return w.pimpl.ConfigInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ConfigList(arg0 context.Context, arg1 types.ConfigListOptions) ([]swarm.Config, error) {
	return w.pimpl.ConfigList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ConfigRemove(arg0 context.Context, arg1 string) error {
	return w.pimpl.ConfigRemove(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ConfigUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.ConfigSpec) error {
	return w.pimpl.ConfigUpdate(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) ContainerAttach(arg0 context.Context, arg1 string, arg2 types.ContainerAttachOptions) (types.HijackedResponse, error) {
	return w.pimpl.ContainerAttach(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerCommit(arg0 context.Context, arg1 string, arg2 types.ContainerCommitOptions) (types.IDResponse, error) {
	return w.pimpl.ContainerCommit(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerCreate(arg0 context.Context, arg1 *container.Config, arg2 *container.HostConfig, arg3 *network.NetworkingConfig, arg4 *v1.Platform, arg5 string) (container.ContainerCreateCreatedBody, error) {
	return w.pimpl.ContainerCreate(arg0, arg1, arg2, arg3, arg4, arg5)
}

func (w clientWithAdditionalCleanup) ContainerDiff(arg0 context.Context, arg1 string) ([]container.ContainerChangeResponseItem, error) {
	return w.pimpl.ContainerDiff(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerExecAttach(arg0 context.Context, arg1 string, arg2 types.ExecStartCheck) (types.HijackedResponse, error) {
	return w.pimpl.ContainerExecAttach(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerExecCreate(arg0 context.Context, arg1 string, arg2 types.ExecConfig) (types.IDResponse, error) {
	return w.pimpl.ContainerExecCreate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerExecInspect(arg0 context.Context, arg1 string) (types.ContainerExecInspect, error) {
	return w.pimpl.ContainerExecInspect(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerExecResize(arg0 context.Context, arg1 string, arg2 types.ResizeOptions) error {
	return w.pimpl.ContainerExecResize(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerExecStart(arg0 context.Context, arg1 string, arg2 types.ExecStartCheck) error {
	return w.pimpl.ContainerExecStart(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerExport(arg0 context.Context, arg1 string) (io.ReadCloser, error) {
	return w.pimpl.ContainerExport(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerInspect(arg0 context.Context, arg1 string) (types.ContainerJSON, error) {
	return w.pimpl.ContainerInspect(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerInspectWithRaw(arg0 context.Context, arg1 string, arg2 bool) (types.ContainerJSON, []uint8, error) {
	return w.pimpl.ContainerInspectWithRaw(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerKill(arg0 context.Context, arg1 string, arg2 string) error {
	return w.pimpl.ContainerKill(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerList(arg0 context.Context, arg1 types.ContainerListOptions) ([]types.Container, error) {
	return w.pimpl.ContainerList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerLogs(arg0 context.Context, arg1 string, arg2 types.ContainerLogsOptions) (io.ReadCloser, error) {
	return w.pimpl.ContainerLogs(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerPause(arg0 context.Context, arg1 string) error {
	return w.pimpl.ContainerPause(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerRemove(arg0 context.Context, arg1 string, arg2 types.ContainerRemoveOptions) error {
	return w.pimpl.ContainerRemove(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerRename(arg0 context.Context, arg1 string, arg2 string) error {
	return w.pimpl.ContainerRename(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerResize(arg0 context.Context, arg1 string, arg2 types.ResizeOptions) error {
	return w.pimpl.ContainerResize(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerRestart(arg0 context.Context, arg1 string, arg2 *time.Duration) error {
	return w.pimpl.ContainerRestart(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerStart(arg0 context.Context, arg1 string, arg2 types.ContainerStartOptions) error {
	return w.pimpl.ContainerStart(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerStatPath(arg0 context.Context, arg1 string, arg2 string) (types.ContainerPathStat, error) {
	return w.pimpl.ContainerStatPath(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerStats(arg0 context.Context, arg1 string, arg2 bool) (types.ContainerStats, error) {
	return w.pimpl.ContainerStats(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerStatsOneShot(arg0 context.Context, arg1 string) (types.ContainerStats, error) {
	return w.pimpl.ContainerStatsOneShot(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerStop(arg0 context.Context, arg1 string, arg2 *time.Duration) error {
	return w.pimpl.ContainerStop(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerTop(arg0 context.Context, arg1 string, arg2 []string) (container.ContainerTopOKBody, error) {
	return w.pimpl.ContainerTop(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerUnpause(arg0 context.Context, arg1 string) error {
	return w.pimpl.ContainerUnpause(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ContainerUpdate(arg0 context.Context, arg1 string, arg2 container.UpdateConfig) (container.ContainerUpdateOKBody, error) {
	return w.pimpl.ContainerUpdate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainerWait(arg0 context.Context, arg1 string, arg2 container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	return w.pimpl.ContainerWait(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ContainersPrune(arg0 context.Context, arg1 filters.Args) (types.ContainersPruneReport, error) {
	return w.pimpl.ContainersPrune(arg0, arg1)
}

func (w clientWithAdditionalCleanup) CopyFromContainer(arg0 context.Context, arg1 string, arg2 string) (io.ReadCloser, types.ContainerPathStat, error) {
	return w.pimpl.CopyFromContainer(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) CopyToContainer(arg0 context.Context, arg1 string, arg2 string, arg3 io.Reader, arg4 types.CopyToContainerOptions) error {
	return w.pimpl.CopyToContainer(arg0, arg1, arg2, arg3, arg4)
}

func (w clientWithAdditionalCleanup) DaemonHost() string {
	return w.pimpl.DaemonHost()
}

func (w clientWithAdditionalCleanup) DialHijack(arg0 context.Context, arg1 string, arg2 string, arg3 map[string][]string) (net.Conn, error) {
	return w.pimpl.DialHijack(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) Dialer() func(context.Context) (net.Conn, error) {
	return w.pimpl.Dialer()
}

func (w clientWithAdditionalCleanup) DiskUsage(arg0 context.Context) (types.DiskUsage, error) {
	return w.pimpl.DiskUsage(arg0)
}

func (w clientWithAdditionalCleanup) DistributionInspect(arg0 context.Context, arg1 string, arg2 string) (registry.DistributionInspect, error) {
	return w.pimpl.DistributionInspect(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) Events(arg0 context.Context, arg1 types.EventsOptions) (<-chan events.Message, <-chan error) {
	return w.pimpl.Events(arg0, arg1)
}

func (w clientWithAdditionalCleanup) HTTPClient() *http.Client {
	return w.pimpl.HTTPClient()
}

func (w clientWithAdditionalCleanup) ImageBuild(arg0 context.Context, arg1 io.Reader, arg2 types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	return w.pimpl.ImageBuild(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImageCreate(arg0 context.Context, arg1 string, arg2 types.ImageCreateOptions) (io.ReadCloser, error) {
	return w.pimpl.ImageCreate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImageHistory(arg0 context.Context, arg1 string) ([]image.HistoryResponseItem, error) {
	return w.pimpl.ImageHistory(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ImageImport(arg0 context.Context, arg1 types.ImageImportSource, arg2 string, arg3 types.ImageImportOptions) (io.ReadCloser, error) {
	return w.pimpl.ImageImport(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) ImageInspectWithRaw(arg0 context.Context, arg1 string) (types.ImageInspect, []uint8, error) {
	return w.pimpl.ImageInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ImageList(arg0 context.Context, arg1 types.ImageListOptions) ([]types.ImageSummary, error) {
	return w.pimpl.ImageList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ImageLoad(arg0 context.Context, arg1 io.Reader, arg2 bool) (types.ImageLoadResponse, error) {
	return w.pimpl.ImageLoad(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImagePull(arg0 context.Context, arg1 string, arg2 types.ImagePullOptions) (io.ReadCloser, error) {
	return w.pimpl.ImagePull(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImagePush(arg0 context.Context, arg1 string, arg2 types.ImagePushOptions) (io.ReadCloser, error) {
	return w.pimpl.ImagePush(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImageRemove(arg0 context.Context, arg1 string, arg2 types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
	return w.pimpl.ImageRemove(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImageSave(arg0 context.Context, arg1 []string) (io.ReadCloser, error) {
	return w.pimpl.ImageSave(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ImageSearch(arg0 context.Context, arg1 string, arg2 types.ImageSearchOptions) ([]registry.SearchResult, error) {
	return w.pimpl.ImageSearch(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImageTag(arg0 context.Context, arg1 string, arg2 string) error {
	return w.pimpl.ImageTag(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ImagesPrune(arg0 context.Context, arg1 filters.Args) (types.ImagesPruneReport, error) {
	return w.pimpl.ImagesPrune(arg0, arg1)
}

func (w clientWithAdditionalCleanup) Info(arg0 context.Context) (types.Info, error) {
	return w.pimpl.Info(arg0)
}

func (w clientWithAdditionalCleanup) NegotiateAPIVersion(arg0 context.Context) {
	w.pimpl.NegotiateAPIVersion(arg0)
}

func (w clientWithAdditionalCleanup) NegotiateAPIVersionPing(arg0 types.Ping) {
	w.pimpl.NegotiateAPIVersionPing(arg0)
}

func (w clientWithAdditionalCleanup) NetworkConnect(arg0 context.Context, arg1 string, arg2 string, arg3 *network.EndpointSettings) error {
	return w.pimpl.NetworkConnect(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) NetworkCreate(arg0 context.Context, arg1 string, arg2 types.NetworkCreate) (types.NetworkCreateResponse, error) {
	return w.pimpl.NetworkCreate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) NetworkDisconnect(arg0 context.Context, arg1 string, arg2 string, arg3 bool) error {
	return w.pimpl.NetworkDisconnect(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) NetworkInspect(arg0 context.Context, arg1 string, arg2 types.NetworkInspectOptions) (types.NetworkResource, error) {
	return w.pimpl.NetworkInspect(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) NetworkInspectWithRaw(arg0 context.Context, arg1 string, arg2 types.NetworkInspectOptions) (types.NetworkResource, []uint8, error) {
	return w.pimpl.NetworkInspectWithRaw(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) NetworkList(arg0 context.Context, arg1 types.NetworkListOptions) ([]types.NetworkResource, error) {
	return w.pimpl.NetworkList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) NetworkRemove(arg0 context.Context, arg1 string) error {
	return w.pimpl.NetworkRemove(arg0, arg1)
}

func (w clientWithAdditionalCleanup) NetworksPrune(arg0 context.Context, arg1 filters.Args) (types.NetworksPruneReport, error) {
	return w.pimpl.NetworksPrune(arg0, arg1)
}

func (w clientWithAdditionalCleanup) NodeInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Node, []uint8, error) {
	return w.pimpl.NodeInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) NodeList(arg0 context.Context, arg1 types.NodeListOptions) ([]swarm.Node, error) {
	return w.pimpl.NodeList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) NodeRemove(arg0 context.Context, arg1 string, arg2 types.NodeRemoveOptions) error {
	return w.pimpl.NodeRemove(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) NodeUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.NodeSpec) error {
	return w.pimpl.NodeUpdate(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) Ping(arg0 context.Context) (types.Ping, error) {
	return w.pimpl.Ping(arg0)
}

func (w clientWithAdditionalCleanup) PluginCreate(arg0 context.Context, arg1 io.Reader, arg2 types.PluginCreateOptions) error {
	return w.pimpl.PluginCreate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginDisable(arg0 context.Context, arg1 string, arg2 types.PluginDisableOptions) error {
	return w.pimpl.PluginDisable(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginEnable(arg0 context.Context, arg1 string, arg2 types.PluginEnableOptions) error {
	return w.pimpl.PluginEnable(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginInspectWithRaw(arg0 context.Context, arg1 string) (*types.Plugin, []uint8, error) {
	return w.pimpl.PluginInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) PluginInstall(arg0 context.Context, arg1 string, arg2 types.PluginInstallOptions) (io.ReadCloser, error) {
	return w.pimpl.PluginInstall(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginList(arg0 context.Context, arg1 filters.Args) (types.PluginsListResponse, error) {
	return w.pimpl.PluginList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) PluginPush(arg0 context.Context, arg1 string, arg2 string) (io.ReadCloser, error) {
	return w.pimpl.PluginPush(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginRemove(arg0 context.Context, arg1 string, arg2 types.PluginRemoveOptions) error {
	return w.pimpl.PluginRemove(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginSet(arg0 context.Context, arg1 string, arg2 []string) error {
	return w.pimpl.PluginSet(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) PluginUpgrade(arg0 context.Context, arg1 string, arg2 types.PluginInstallOptions) (io.ReadCloser, error) {
	return w.pimpl.PluginUpgrade(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) RegistryLogin(arg0 context.Context, arg1 types.AuthConfig) (registry.AuthenticateOKBody, error) {
	return w.pimpl.RegistryLogin(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SecretCreate(arg0 context.Context, arg1 swarm.SecretSpec) (types.SecretCreateResponse, error) {
	return w.pimpl.SecretCreate(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SecretInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Secret, []uint8, error) {
	return w.pimpl.SecretInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SecretList(arg0 context.Context, arg1 types.SecretListOptions) ([]swarm.Secret, error) {
	return w.pimpl.SecretList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SecretRemove(arg0 context.Context, arg1 string) error {
	return w.pimpl.SecretRemove(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SecretUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.SecretSpec) error {
	return w.pimpl.SecretUpdate(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) ServerVersion(arg0 context.Context) (types.Version, error) {
	return w.pimpl.ServerVersion(arg0)
}

func (w clientWithAdditionalCleanup) ServiceCreate(arg0 context.Context, arg1 swarm.ServiceSpec, arg2 types.ServiceCreateOptions) (types.ServiceCreateResponse, error) {
	return w.pimpl.ServiceCreate(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ServiceInspectWithRaw(arg0 context.Context, arg1 string, arg2 types.ServiceInspectOptions) (swarm.Service, []uint8, error) {
	return w.pimpl.ServiceInspectWithRaw(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ServiceList(arg0 context.Context, arg1 types.ServiceListOptions) ([]swarm.Service, error) {
	return w.pimpl.ServiceList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ServiceLogs(arg0 context.Context, arg1 string, arg2 types.ContainerLogsOptions) (io.ReadCloser, error) {
	return w.pimpl.ServiceLogs(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) ServiceRemove(arg0 context.Context, arg1 string) error {
	return w.pimpl.ServiceRemove(arg0, arg1)
}

func (w clientWithAdditionalCleanup) ServiceUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.ServiceSpec, arg4 types.ServiceUpdateOptions) (types.ServiceUpdateResponse, error) {
	return w.pimpl.ServiceUpdate(arg0, arg1, arg2, arg3, arg4)
}

func (w clientWithAdditionalCleanup) SwarmGetUnlockKey(arg0 context.Context) (types.SwarmUnlockKeyResponse, error) {
	return w.pimpl.SwarmGetUnlockKey(arg0)
}

func (w clientWithAdditionalCleanup) SwarmInit(arg0 context.Context, arg1 swarm.InitRequest) (string, error) {
	return w.pimpl.SwarmInit(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SwarmInspect(arg0 context.Context) (swarm.Swarm, error) {
	return w.pimpl.SwarmInspect(arg0)
}

func (w clientWithAdditionalCleanup) SwarmJoin(arg0 context.Context, arg1 swarm.JoinRequest) error {
	return w.pimpl.SwarmJoin(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SwarmLeave(arg0 context.Context, arg1 bool) error {
	return w.pimpl.SwarmLeave(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SwarmUnlock(arg0 context.Context, arg1 swarm.UnlockRequest) error {
	return w.pimpl.SwarmUnlock(arg0, arg1)
}

func (w clientWithAdditionalCleanup) SwarmUpdate(arg0 context.Context, arg1 swarm.Version, arg2 swarm.Spec, arg3 swarm.UpdateFlags) error {
	return w.pimpl.SwarmUpdate(arg0, arg1, arg2, arg3)
}

func (w clientWithAdditionalCleanup) TaskInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Task, []uint8, error) {
	return w.pimpl.TaskInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) TaskList(arg0 context.Context, arg1 types.TaskListOptions) ([]swarm.Task, error) {
	return w.pimpl.TaskList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) TaskLogs(arg0 context.Context, arg1 string, arg2 types.ContainerLogsOptions) (io.ReadCloser, error) {
	return w.pimpl.TaskLogs(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) VolumeCreate(arg0 context.Context, arg1 volume.VolumeCreateBody) (types.Volume, error) {
	return w.pimpl.VolumeCreate(arg0, arg1)
}

func (w clientWithAdditionalCleanup) VolumeInspect(arg0 context.Context, arg1 string) (types.Volume, error) {
	return w.pimpl.VolumeInspect(arg0, arg1)
}

func (w clientWithAdditionalCleanup) VolumeInspectWithRaw(arg0 context.Context, arg1 string) (types.Volume, []uint8, error) {
	return w.pimpl.VolumeInspectWithRaw(arg0, arg1)
}

func (w clientWithAdditionalCleanup) VolumeList(arg0 context.Context, arg1 filters.Args) (volume.VolumeListOKBody, error) {
	return w.pimpl.VolumeList(arg0, arg1)
}

func (w clientWithAdditionalCleanup) VolumeRemove(arg0 context.Context, arg1 string, arg2 bool) error {
	return w.pimpl.VolumeRemove(arg0, arg1, arg2)
}

func (w clientWithAdditionalCleanup) VolumesPrune(arg0 context.Context, arg1 filters.Args) (types.VolumesPruneReport, error) {
	return w.pimpl.VolumesPrune(arg0, arg1)
}

//</editor-fold>
