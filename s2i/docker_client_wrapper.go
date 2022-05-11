package s2i

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// Wrapper to workaround https://github.com/containers/podman/issues/13770
type podmanDockerClient struct {
	client.CommonAPIClient
}

func (p podmanDockerClient) ContainerCommit(ctx context.Context, nameOrID string, opts types.ContainerCommitOptions) (types.IDResponse, error) {
	if len(opts.Config.Cmd) > 0 {
		bs, err := json.Marshal(opts.Config.Cmd)
		if err != nil {
			return types.IDResponse{}, err
		}
		opts.Changes = append(opts.Changes, "CMD "+string(bs))
	}

	if len(opts.Config.Entrypoint) > 0 {
		bs, err := json.Marshal(opts.Config.Entrypoint)
		if err != nil {
			return types.IDResponse{}, err
		}
		opts.Changes = append(opts.Changes, "ENTRYPOINT "+string(bs))
	}

	if opts.Config.User != "" {
		opts.Changes = append(opts.Changes, "USER "+opts.Config.User)
	}

	for _, e := range opts.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		opts.Changes = append(opts.Changes, fmt.Sprintf("ENV %s=%q", parts[0], parts[1]))
	}

	for k, v := range opts.Config.Labels {
		opts.Changes = append(opts.Changes, fmt.Sprintf("LABEL %q=%q", k, v))
	}

	return p.CommonAPIClient.ContainerCommit(ctx, nameOrID, opts)
}

func isPodman(ctx context.Context, cli client.CommonAPIClient) bool {
	v, err := cli.ServerVersion(ctx)
	if err != nil {
		return false
	}
	for _, comp := range v.Components {
		if comp.Name == "Podman Engine" {
			return true
		}
	}
	return false
}
