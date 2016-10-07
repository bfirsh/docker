package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types"
)

type mockFunc func(*http.Request) (*http.Response, error)

// mockFuncGenerator will return a series of mock functions, so that multiple
// HTTP calls can be tested with newMockClient
func mockFuncGenerator(mockFuncs []mockFunc) mockFunc {
	i := 0
	return func(req *http.Request) (*http.Response, error) {
		if i >= len(mockFuncs) {
			return nil, fmt.Errorf("unexpected HTTP call: %s %s", req.Method, req.URL)
		}
		f := mockFuncs[i]
		i++
		return f(req)
	}
}

func newMockClient(doer mockFunc) *http.Client {
	return &http.Client{
		Transport: transportFunc(doer),
	}
}

// responseMock mocks a standard response
func responseMock(expectedMethod, expectedPath string, responseStatus int, responseBody interface{}) mockFunc {
	return func(req *http.Request) (*http.Response, error) {
		if req.Method != expectedMethod {
			return nil, fmt.Errorf("Expected method '%s', got '%s'", expectedMethod, req.Method)
		}
		path := strings.TrimPrefix(req.URL.Path, "http://")
		if !strings.HasPrefix(path, expectedPath) {
			return nil, fmt.Errorf("Expected URL '%s', got '%s'", expectedPath, req.URL)
		}
		b := []byte{}
		if responseBody != nil {
			if s, ok := responseBody.(string); ok {
				// pass strings straight to response
				b = []byte(s)
			} else {
				// encode objects as JSON
				var err error
				b, err = json.Marshal(responseBody)
				if err != nil {
					return nil, err
				}
			}
		}
		return &http.Response{
			StatusCode: responseStatus,
			Body:       ioutil.NopCloser(bytes.NewReader(b)),
		}, nil
	}
}
func errorMock(statusCode int, message string) mockFunc {
	return func(req *http.Request) (*http.Response, error) {
		header := http.Header{}
		header.Set("Content-Type", "application/json")

		body, err := json.Marshal(&types.ErrorResponse{
			Message: message,
		})
		if err != nil {
			return nil, err
		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
			Header:     header,
		}, nil
	}
}

func plainTextErrorMock(statusCode int, message string) mockFunc {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(message))),
		}, nil
	}
}
