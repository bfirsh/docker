package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"golang.org/x/net/context"
)

type cidFile struct {
	path    string
	file    *os.File
	written bool
}

func (cid *cidFile) Close() error {
	cid.file.Close()

	if !cid.written {
		if err := os.Remove(cid.path); err != nil {
			return fmt.Errorf("failed to remove the CID file '%s': %s \n", cid.path, err)
		}
	}

	return nil
}

func (cid *cidFile) Write(id string) error {
	if _, err := cid.file.Write([]byte(id)); err != nil {
		return fmt.Errorf("Failed to write the container ID to the file: %s", err)
	}
	cid.written = true
	return nil
}

func newCIDFile(path string) (*cidFile, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("Container ID file found, make sure the other container isn't running or delete %s", path)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to create the container ID file: %s", err)
	}

	return &cidFile{path: path, file: f}, nil
}

type configWrapper struct {
	*container.Config
	HostConfig       *container.HostConfig
	NetworkingConfig *network.NetworkingConfig
}

// ContainerCreate creates a new container based in the given configuration.
// It can be associated with a name, but it's not mandatory.
func (cli *Client) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (types.ContainerCreateResponse, error) {
	var response types.ContainerCreateResponse
	query := url.Values{}
	if containerName != "" {
		query.Set("name", containerName)
	}

	body := configWrapper{
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkingConfig,
	}

	var containerIDFile *cidFile
	if hostConfig.ContainerIDFile != "" {
		var err error
		if containerIDFile, err = newCIDFile(hostConfig.ContainerIDFile); err != nil {
			return response, err
		}
		defer containerIDFile.Close()
	}

	serverResp, err := cli.post(ctx, "/containers/create", query, body, nil)
	if err != nil {
		if serverResp.statusCode == 404 && strings.Contains(err.Error(), "No such image") {
			return response, imageNotFoundError{config.Image}
		}
		return response, err
	}

	err = json.NewDecoder(serverResp.body).Decode(&response)
	ensureReaderClosed(serverResp)

	if err == nil && containerIDFile != nil {
		if err = containerIDFile.Write(response.ID); err != nil {
			return response, err
		}
	}
	return response, err
}
