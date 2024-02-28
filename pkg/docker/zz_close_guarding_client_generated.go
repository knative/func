package docker

import (
	"context"
	"io"
	"net"
	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func (c *closeGuardingClient) BuildCachePrune(arg0 context.Context, arg1 types.BuildCachePruneOptions) (*types.BuildCachePruneReport, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.BuildCachePrune(arg0, arg1)
}

func (c *closeGuardingClient) BuildCancel(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.BuildCancel(arg0, arg1)
}

func (c *closeGuardingClient) ClientVersion() string {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ClientVersion()
}

func (c *closeGuardingClient) ConfigCreate(arg0 context.Context, arg1 swarm.ConfigSpec) (types.ConfigCreateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ConfigCreate(arg0, arg1)
}

func (c *closeGuardingClient) ConfigInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Config, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ConfigInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) ConfigList(arg0 context.Context, arg1 types.ConfigListOptions) ([]swarm.Config, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ConfigList(arg0, arg1)
}

func (c *closeGuardingClient) ConfigRemove(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ConfigRemove(arg0, arg1)
}

func (c *closeGuardingClient) ConfigUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.ConfigSpec) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ConfigUpdate(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) ContainerAttach(arg0 context.Context, arg1 string, arg2 container.AttachOptions) (types.HijackedResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerAttach(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerCommit(arg0 context.Context, arg1 string, arg2 container.CommitOptions) (types.IDResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerCommit(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerCreate(arg0 context.Context, arg1 *container.Config, arg2 *container.HostConfig, arg3 *network.NetworkingConfig, arg4 *v1.Platform, arg5 string) (container.CreateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerCreate(arg0, arg1, arg2, arg3, arg4, arg5)
}

func (c *closeGuardingClient) ContainerDiff(arg0 context.Context, arg1 string) ([]container.FilesystemChange, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerDiff(arg0, arg1)
}

func (c *closeGuardingClient) ContainerExecAttach(arg0 context.Context, arg1 string, arg2 types.ExecStartCheck) (types.HijackedResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExecAttach(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerExecCreate(arg0 context.Context, arg1 string, arg2 types.ExecConfig) (types.IDResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExecCreate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerExecInspect(arg0 context.Context, arg1 string) (types.ContainerExecInspect, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExecInspect(arg0, arg1)
}

func (c *closeGuardingClient) ContainerExecResize(arg0 context.Context, arg1 string, arg2 container.ResizeOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExecResize(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerExecStart(arg0 context.Context, arg1 string, arg2 types.ExecStartCheck) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExecStart(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerExport(arg0 context.Context, arg1 string) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerExport(arg0, arg1)
}

func (c *closeGuardingClient) ContainerInspect(arg0 context.Context, arg1 string) (types.ContainerJSON, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerInspect(arg0, arg1)
}

func (c *closeGuardingClient) ContainerInspectWithRaw(arg0 context.Context, arg1 string, arg2 bool) (types.ContainerJSON, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerInspectWithRaw(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerKill(arg0 context.Context, arg1 string, arg2 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerKill(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerList(arg0 context.Context, arg1 container.ListOptions) ([]types.Container, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerList(arg0, arg1)
}

func (c *closeGuardingClient) ContainerLogs(arg0 context.Context, arg1 string, arg2 container.LogsOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerLogs(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerPause(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerPause(arg0, arg1)
}

func (c *closeGuardingClient) ContainerRemove(arg0 context.Context, arg1 string, arg2 container.RemoveOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerRemove(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerRename(arg0 context.Context, arg1 string, arg2 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerRename(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerResize(arg0 context.Context, arg1 string, arg2 container.ResizeOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerResize(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerRestart(arg0 context.Context, arg1 string, arg2 container.StopOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerRestart(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerStart(arg0 context.Context, arg1 string, arg2 container.StartOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerStart(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerStatPath(arg0 context.Context, arg1 string, arg2 string) (types.ContainerPathStat, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerStatPath(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerStats(arg0 context.Context, arg1 string, arg2 bool) (types.ContainerStats, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerStats(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerStatsOneShot(arg0 context.Context, arg1 string) (types.ContainerStats, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerStatsOneShot(arg0, arg1)
}

func (c *closeGuardingClient) ContainerStop(arg0 context.Context, arg1 string, arg2 container.StopOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerStop(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerTop(arg0 context.Context, arg1 string, arg2 []string) (container.ContainerTopOKBody, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerTop(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerUnpause(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerUnpause(arg0, arg1)
}

func (c *closeGuardingClient) ContainerUpdate(arg0 context.Context, arg1 string, arg2 container.UpdateConfig) (container.ContainerUpdateOKBody, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerUpdate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainerWait(arg0 context.Context, arg1 string, arg2 container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainerWait(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ContainersPrune(arg0 context.Context, arg1 filters.Args) (types.ContainersPruneReport, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ContainersPrune(arg0, arg1)
}

func (c *closeGuardingClient) CopyFromContainer(arg0 context.Context, arg1 string, arg2 string) (io.ReadCloser, types.ContainerPathStat, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.CopyFromContainer(arg0, arg1, arg2)
}

func (c *closeGuardingClient) CopyToContainer(arg0 context.Context, arg1 string, arg2 string, arg3 io.Reader, arg4 types.CopyToContainerOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.CopyToContainer(arg0, arg1, arg2, arg3, arg4)
}

func (c *closeGuardingClient) DaemonHost() string {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.DaemonHost()
}

func (c *closeGuardingClient) DialHijack(arg0 context.Context, arg1 string, arg2 string, arg3 map[string][]string) (net.Conn, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.DialHijack(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) Dialer() func(context.Context) (net.Conn, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.Dialer()
}

func (c *closeGuardingClient) DiskUsage(arg0 context.Context, arg1 types.DiskUsageOptions) (types.DiskUsage, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.DiskUsage(arg0, arg1)
}

func (c *closeGuardingClient) DistributionInspect(arg0 context.Context, arg1 string, arg2 string) (registry.DistributionInspect, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.DistributionInspect(arg0, arg1, arg2)
}

func (c *closeGuardingClient) Events(arg0 context.Context, arg1 types.EventsOptions) (<-chan events.Message, <-chan error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.Events(arg0, arg1)
}

func (c *closeGuardingClient) HTTPClient() *http.Client {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.HTTPClient()
}

func (c *closeGuardingClient) ImageBuild(arg0 context.Context, arg1 io.Reader, arg2 types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageBuild(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImageCreate(arg0 context.Context, arg1 string, arg2 types.ImageCreateOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageCreate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImageHistory(arg0 context.Context, arg1 string) ([]image.HistoryResponseItem, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageHistory(arg0, arg1)
}

func (c *closeGuardingClient) ImageImport(arg0 context.Context, arg1 types.ImageImportSource, arg2 string, arg3 types.ImageImportOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageImport(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) ImageInspectWithRaw(arg0 context.Context, arg1 string) (types.ImageInspect, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) ImageList(arg0 context.Context, arg1 types.ImageListOptions) ([]image.Summary, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageList(arg0, arg1)
}

func (c *closeGuardingClient) ImageLoad(arg0 context.Context, arg1 io.Reader, arg2 bool) (types.ImageLoadResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageLoad(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImagePull(arg0 context.Context, arg1 string, arg2 types.ImagePullOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImagePull(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImagePush(arg0 context.Context, arg1 string, arg2 types.ImagePushOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImagePush(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImageRemove(arg0 context.Context, arg1 string, arg2 types.ImageRemoveOptions) ([]image.DeleteResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageRemove(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImageSave(arg0 context.Context, arg1 []string) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageSave(arg0, arg1)
}

func (c *closeGuardingClient) ImageSearch(arg0 context.Context, arg1 string, arg2 types.ImageSearchOptions) ([]registry.SearchResult, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageSearch(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImageTag(arg0 context.Context, arg1 string, arg2 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImageTag(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ImagesPrune(arg0 context.Context, arg1 filters.Args) (types.ImagesPruneReport, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ImagesPrune(arg0, arg1)
}

func (c *closeGuardingClient) Info(arg0 context.Context) (system.Info, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.Info(arg0)
}

func (c *closeGuardingClient) NegotiateAPIVersion(arg0 context.Context) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	c.pimpl.NegotiateAPIVersion(arg0)
}

func (c *closeGuardingClient) NegotiateAPIVersionPing(arg0 types.Ping) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	c.pimpl.NegotiateAPIVersionPing(arg0)
}

func (c *closeGuardingClient) NetworkConnect(arg0 context.Context, arg1 string, arg2 string, arg3 *network.EndpointSettings) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkConnect(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) NetworkCreate(arg0 context.Context, arg1 string, arg2 types.NetworkCreate) (types.NetworkCreateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkCreate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) NetworkDisconnect(arg0 context.Context, arg1 string, arg2 string, arg3 bool) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkDisconnect(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) NetworkInspect(arg0 context.Context, arg1 string, arg2 types.NetworkInspectOptions) (types.NetworkResource, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkInspect(arg0, arg1, arg2)
}

func (c *closeGuardingClient) NetworkInspectWithRaw(arg0 context.Context, arg1 string, arg2 types.NetworkInspectOptions) (types.NetworkResource, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkInspectWithRaw(arg0, arg1, arg2)
}

func (c *closeGuardingClient) NetworkList(arg0 context.Context, arg1 types.NetworkListOptions) ([]types.NetworkResource, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkList(arg0, arg1)
}

func (c *closeGuardingClient) NetworkRemove(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworkRemove(arg0, arg1)
}

func (c *closeGuardingClient) NetworksPrune(arg0 context.Context, arg1 filters.Args) (types.NetworksPruneReport, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NetworksPrune(arg0, arg1)
}

func (c *closeGuardingClient) NodeInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Node, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NodeInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) NodeList(arg0 context.Context, arg1 types.NodeListOptions) ([]swarm.Node, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NodeList(arg0, arg1)
}

func (c *closeGuardingClient) NodeRemove(arg0 context.Context, arg1 string, arg2 types.NodeRemoveOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NodeRemove(arg0, arg1, arg2)
}

func (c *closeGuardingClient) NodeUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.NodeSpec) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.NodeUpdate(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) Ping(arg0 context.Context) (types.Ping, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.Ping(arg0)
}

func (c *closeGuardingClient) PluginCreate(arg0 context.Context, arg1 io.Reader, arg2 types.PluginCreateOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginCreate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginDisable(arg0 context.Context, arg1 string, arg2 types.PluginDisableOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginDisable(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginEnable(arg0 context.Context, arg1 string, arg2 types.PluginEnableOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginEnable(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginInspectWithRaw(arg0 context.Context, arg1 string) (*types.Plugin, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) PluginInstall(arg0 context.Context, arg1 string, arg2 types.PluginInstallOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginInstall(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginList(arg0 context.Context, arg1 filters.Args) (types.PluginsListResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginList(arg0, arg1)
}

func (c *closeGuardingClient) PluginPush(arg0 context.Context, arg1 string, arg2 string) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginPush(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginRemove(arg0 context.Context, arg1 string, arg2 types.PluginRemoveOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginRemove(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginSet(arg0 context.Context, arg1 string, arg2 []string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginSet(arg0, arg1, arg2)
}

func (c *closeGuardingClient) PluginUpgrade(arg0 context.Context, arg1 string, arg2 types.PluginInstallOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.PluginUpgrade(arg0, arg1, arg2)
}

func (c *closeGuardingClient) RegistryLogin(arg0 context.Context, arg1 registry.AuthConfig) (registry.AuthenticateOKBody, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.RegistryLogin(arg0, arg1)
}

func (c *closeGuardingClient) SecretCreate(arg0 context.Context, arg1 swarm.SecretSpec) (types.SecretCreateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SecretCreate(arg0, arg1)
}

func (c *closeGuardingClient) SecretInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Secret, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SecretInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) SecretList(arg0 context.Context, arg1 types.SecretListOptions) ([]swarm.Secret, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SecretList(arg0, arg1)
}

func (c *closeGuardingClient) SecretRemove(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SecretRemove(arg0, arg1)
}

func (c *closeGuardingClient) SecretUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.SecretSpec) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SecretUpdate(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) ServerVersion(arg0 context.Context) (types.Version, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServerVersion(arg0)
}

func (c *closeGuardingClient) ServiceCreate(arg0 context.Context, arg1 swarm.ServiceSpec, arg2 types.ServiceCreateOptions) (swarm.ServiceCreateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceCreate(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ServiceInspectWithRaw(arg0 context.Context, arg1 string, arg2 types.ServiceInspectOptions) (swarm.Service, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceInspectWithRaw(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ServiceList(arg0 context.Context, arg1 types.ServiceListOptions) ([]swarm.Service, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceList(arg0, arg1)
}

func (c *closeGuardingClient) ServiceLogs(arg0 context.Context, arg1 string, arg2 container.LogsOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceLogs(arg0, arg1, arg2)
}

func (c *closeGuardingClient) ServiceRemove(arg0 context.Context, arg1 string) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceRemove(arg0, arg1)
}

func (c *closeGuardingClient) ServiceUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 swarm.ServiceSpec, arg4 types.ServiceUpdateOptions) (swarm.ServiceUpdateResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.ServiceUpdate(arg0, arg1, arg2, arg3, arg4)
}

func (c *closeGuardingClient) SwarmGetUnlockKey(arg0 context.Context) (types.SwarmUnlockKeyResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmGetUnlockKey(arg0)
}

func (c *closeGuardingClient) SwarmInit(arg0 context.Context, arg1 swarm.InitRequest) (string, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmInit(arg0, arg1)
}

func (c *closeGuardingClient) SwarmInspect(arg0 context.Context) (swarm.Swarm, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmInspect(arg0)
}

func (c *closeGuardingClient) SwarmJoin(arg0 context.Context, arg1 swarm.JoinRequest) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmJoin(arg0, arg1)
}

func (c *closeGuardingClient) SwarmLeave(arg0 context.Context, arg1 bool) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmLeave(arg0, arg1)
}

func (c *closeGuardingClient) SwarmUnlock(arg0 context.Context, arg1 swarm.UnlockRequest) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmUnlock(arg0, arg1)
}

func (c *closeGuardingClient) SwarmUpdate(arg0 context.Context, arg1 swarm.Version, arg2 swarm.Spec, arg3 swarm.UpdateFlags) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.SwarmUpdate(arg0, arg1, arg2, arg3)
}

func (c *closeGuardingClient) TaskInspectWithRaw(arg0 context.Context, arg1 string) (swarm.Task, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.TaskInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) TaskList(arg0 context.Context, arg1 types.TaskListOptions) ([]swarm.Task, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.TaskList(arg0, arg1)
}

func (c *closeGuardingClient) TaskLogs(arg0 context.Context, arg1 string, arg2 container.LogsOptions) (io.ReadCloser, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.TaskLogs(arg0, arg1, arg2)
}

func (c *closeGuardingClient) VolumeCreate(arg0 context.Context, arg1 volume.CreateOptions) (volume.Volume, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeCreate(arg0, arg1)
}

func (c *closeGuardingClient) VolumeInspect(arg0 context.Context, arg1 string) (volume.Volume, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeInspect(arg0, arg1)
}

func (c *closeGuardingClient) VolumeInspectWithRaw(arg0 context.Context, arg1 string) (volume.Volume, []uint8, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeInspectWithRaw(arg0, arg1)
}

func (c *closeGuardingClient) VolumeList(arg0 context.Context, arg1 volume.ListOptions) (volume.ListResponse, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeList(arg0, arg1)
}

func (c *closeGuardingClient) VolumeRemove(arg0 context.Context, arg1 string, arg2 bool) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeRemove(arg0, arg1, arg2)
}

func (c *closeGuardingClient) VolumesPrune(arg0 context.Context, arg1 filters.Args) (types.VolumesPruneReport, error) {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumesPrune(arg0, arg1)
}

func (c *closeGuardingClient) VolumeUpdate(arg0 context.Context, arg1 string, arg2 swarm.Version, arg3 volume.UpdateOptions) error {
	c.m.RLock()
	defer c.m.RUnlock()
	if c.closed {
		panic("use of closed client")
	}
	return c.pimpl.VolumeUpdate(arg0, arg1, arg2, arg3)
}
