package client

import (
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"golang.org/x/net/context"
)

func (cli *Client) ContainerRun(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (io.ReadCloser, error) {
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, containerName)
	if err != nil {
		if IsErrImageNotFound(err) {
			if _, err = cli.ImageCreate(ctx, config.Image, types.ImageCreateOptions{}); err != nil {
				return nil, err
			}
			if resp, err = cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, containerName); err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, err
	}
	if _, err = cli.ContainerWait(ctx, resp.ID); err != nil {
		return nil, err
	}
	return cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
}
