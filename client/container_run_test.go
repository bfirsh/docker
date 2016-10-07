package client

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/testutil/assert"
	"golang.org/x/net/context"
)

func TestContainerRun(t *testing.T) {
	client := &Client{
		client: newMockClient(mockFuncGenerator([]mockFunc{
			responseMock("POST", "/containers/create",
				http.StatusOK, types.ContainerCreateResponse{ID: "container_id"}),
			responseMock("POST", "/containers/container_id/start",
				http.StatusOK, nil),
			responseMock("POST", "/containers/container_id/wait",
				http.StatusOK, types.ContainerWaitResponse{StatusCode: 0}),
			responseMock("GET", "/containers/container_id/logs",
				http.StatusOK, "hello world\n"),
		})),
	}
	out, err := client.ContainerRun(context.Background(), &container.Config{Image: "alpine", Cmd: []string{"echo", "hello world"}}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, strings.TrimSpace(string(b)), "hello world")
}

func TestContainerRunWithImagePull(t *testing.T) {
	client := &Client{
		client: newMockClient(mockFuncGenerator([]mockFunc{
			errorMock(http.StatusNotFound, "No such image"),
			responseMock("POST", "/containers/container_id/pull",
				http.StatusOK, "hello world\n"),
			responseMock("POST", "/images/create?fromImage=alpine",
				http.StatusOK, types.ContainerCreateResponse{ID: "container_id"}),
			responseMock("POST", "/containers/container_id/start",
				http.StatusOK, nil),
			responseMock("POST", "/containers/container_id/wait",
				http.StatusOK, types.ContainerWaitResponse{StatusCode: 0}),
			responseMock("GET", "/containers/container_id/logs",
				http.StatusOK, "hello world\n"),
		})),
	}
	out, err := client.ContainerRun(context.Background(), &container.Config{Image: "alpine", Cmd: []string{"echo", "hello world"}}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, strings.TrimSpace(string(b)), "hello world")
}
