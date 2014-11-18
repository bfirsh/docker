package hosts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/pkg/log"
)

var (
	validHostNameChars   = `[a-zA-Z0-9_]`
	validHostNamePattern = regexp.MustCompile(`^` + validHostNameChars + `+$`)
)

type Host struct {
	Name       string `json:"-"`
	DriverName string
	Driver     drivers.Driver
	storePath  string
}

type hostConfig struct {
	DriverName string
}

type ConnectionDetails struct {
	Proto string
	Addr  string
}

func NewHost(name, driverName, storePath string) (*Host, error) {
	driver, err := drivers.NewDriver(driverName, storePath)
	if err != nil {
		return nil, err
	}
	return &Host{
		Name:       name,
		DriverName: driverName,
		Driver:     driver,
		storePath:  storePath,
	}, nil
}

func NewDefaultHost(url string) *Host {
	host := &Host{Name: "default"}
	host.Driver = &drivers.DefaultDriver{URL: url}
	return host
}

func LoadHost(name string, storePath string) (*Host, error) {
	if name == "default" {
		return NewDefaultHost(""), nil
	}

	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Host %q does not exist", name)
	}

	host := &Host{Name: name, storePath: storePath}
	if err := host.LoadConfig(); err != nil {
		return nil, err
	}
	return host, nil
}

func ValidateHostName(name string) (string, error) {
	if !validHostNamePattern.MatchString(name) {
		return name, fmt.Errorf("Invalid host name %q, it must match %s", name, validHostNamePattern)
	}
	return name, nil
}

func (h *Host) Create() error {
	if err := h.Driver.Create(); err != nil {
		if rmErr := h.removeStorePath(); rmErr != nil {
			log.Errorf("Error cleaning up: %s", rmErr)
		}
		return err
	}
	if err := h.SaveConfig(); err != nil {
		return err
	}
	return nil
}

func (h *Host) Start() error {
	return h.Driver.Start()
}

func (h *Host) Stop() error {
	return h.Driver.Stop()
}

func (h *Host) Remove(force bool) error {
	if err := h.Driver.Remove(); err != nil {
		if force {
			log.Errorf("Error removing host, force removing anyway: %s", err)
		} else {
			return err
		}
	}
	return h.removeStorePath()
}

func (h *Host) removeStorePath() error {
	file, err := os.Stat(h.storePath)
	if err != nil {
		return err
	}
	if !file.IsDir() {
		return fmt.Errorf("%q is not a directory", h.storePath)
	}
	return os.RemoveAll(h.storePath)
}

func (h *Host) GetURL() (string, error) {
	return h.Driver.GetURL()
}

// GetProtoAddr returns the protocol and address based on the URL
func (h *Host) GetProtoAddr() (proto, addr string, err error) {
	url, err := h.GetURL()
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(url, "://", 2)
	if len(parts) == 1 {
		return "", "", fmt.Errorf("The URL for host %q is not valid: %s", h.Name, url)
	}
	return parts[0], parts[1], nil
}

func (h *Host) LoadConfig() error {
	data, err := ioutil.ReadFile(path.Join(h.storePath, "config.json"))
	if err != nil {
		return err
	}

	// First pass: find the driver name and load the driver
	var config hostConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	driver, err := drivers.NewDriver(config.DriverName, h.storePath)
	if err != nil {
		return err
	}
	h.Driver = driver

	// Second pass: unmarshal driver config into correct driver
	if err := json.Unmarshal(data, &h); err != nil {
		return err
	}

	return nil
}

func (h *Host) SaveConfig() error {
	if h.Name == "default" {
		return fmt.Errorf("Default host's config cannot be saved")
	}
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(h.storePath, "config.json"), data, 0600); err != nil {
		return err
	}
	return nil
}
