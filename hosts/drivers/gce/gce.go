package gce

import (
	"fmt"

	"github.com/docker/docker/hosts/state"

	"os"
	"os/exec"
	"path"
	"runtime"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	flag "github.com/docker/docker/pkg/mflag"
)

// Driver is a struct compatible with the docker.hosts.drivers.Driver interface.
type Driver struct {
	InstanceName     string
	Zone             string
	MachineType      string
	storePath        string
	UserName         string
	Project          string
	sshKeyPath       string
	publicSSHKeyPath string
}

// CreateFlags are the command line flags used to create a driver.
type CreateFlags struct {
	InstanceName *string
	Zone         *string
	MachineType  *string
	UserName     *string
	Project      *string
}

func init() {
	drivers.Register("gce", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)

	createFlags.Zone = cmd.String(
		[]string{"-gce-zone"},
		"us-central1-a",
		"GCE location",
	)
	createFlags.MachineType = cmd.String(
		[]string{"-gce-machine-type"},
		"f1-micro",
		"GCE machine type",
	)
	createFlags.UserName = cmd.String(
		[]string{"-gce-username"},
		"",
		"GCE username",
	)
	createFlags.InstanceName = cmd.String(
		[]string{"-gce-instance-name"},
		"docker-host",
		"GCE Instance name",
	)
	createFlags.Project = cmd.String(
		[]string{"-gce-project"},
		"",
		"GCE Project name",
	)
	return createFlags
}

// NewDriver creates a Driver with the specified storePath.
func NewDriver(storePath string) (drivers.Driver, error) {
	driver := &Driver{storePath: storePath}
	driver.sshKeyPath = path.Join(storePath, "id_rsa")
	driver.publicSSHKeyPath = path.Join(storePath, "id_rsa.pub")
	return driver, nil
}

// DriverName returns the name of the driver.
func (driver *Driver) DriverName() string {
	return "gce"
}

// SetConfigFromFlags initializes the driver based on the command line flags.
func (driver *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)

	driver.InstanceName = *flags.InstanceName
	driver.Zone = *flags.Zone
	driver.MachineType = *flags.MachineType
	if *flags.UserName == "" {
		if runtime.GOOS == "windows" {
			driver.UserName = os.Getenv("USERNAME")
		} else {
			driver.UserName = os.Getenv("USER")
		}
	}
	if driver.UserName == "" {
		return fmt.Errorf("Unable to determine the current username.")
	}
	if *flags.Project == "" {
		return fmt.Errorf("Please specify the GCE Project name using the option --gce-project.")
	}
	driver.Project = *flags.Project

	return nil
}

func (driver *Driver) initApis() (*ComputeUtil, error) {
	return newComputeUtil(driver)
}

// Create creates a GCE VM instance acting as a docker host.
func (driver *Driver) Create() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	log.Infof("Creating GCE host...")
	// Check if the instance already exists.
	instance, _ := c.instance()
	if instance != nil {
		return fmt.Errorf("Instance %v already exists.", driver.InstanceName)
	}

	log.Infof("Generating SSH Key")
	if err := ssh.GenerateSSHKey(driver.sshKeyPath); err != nil {
		return err
	}

	return c.createInstance(driver.publicSSHKeyPath, driver.sshKeyPath, driver.MachineType)
}

// GetURL returns the URL of the remote docker daemon.
func (driver *Driver) GetURL() (string, error) {
	ip, err := driver.GetIP()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("tcp://%s:2375", ip)
	return url, nil
}

// GetIP returns the IP address of the GCE instance.
func (driver *Driver) GetIP() (string, error) {
	c, err := newComputeUtil(driver)
	if err != nil {
		return "", err
	}
	return c.ip()
}

// GetState returns a docker.hosts.state.State value representing the current state of the host.
func (driver *Driver) GetState() (state.State, error) {
	c, err := newComputeUtil(driver)
	if err != nil {
		return state.None, err
	}
	disk, _ := c.disk()
	instance, _ := c.instance()
	if instance == nil && disk == nil {
		return state.None, nil
	}
	if instance == nil && disk != nil {
		return state.Stopped, nil
	}

	switch instance.Status {
	case "PROVISIONING", "STAGING":
		return state.Starting, nil
	case "RUNNING":
		return state.Running, nil
	case "STOPPING", "STOPPED", "TERMINATED":
		return state.Stopped, nil
	}
	return state.None, nil
}

// Start creates a GCE instance and attaches it to the existing disk.
func (driver *Driver) Start() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	return c.createInstance(driver.publicSSHKeyPath, driver.sshKeyPath, driver.MachineType)
}

// Stop deletes the GCE instance, but keeps the disk.
func (driver *Driver) Stop() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	return c.deleteInstance()
}

// Remove deletes the GCE instance and the disk.
func (driver *Driver) Remove() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	s, err := driver.GetState()
	if err != nil {
		return err
	}
	if s == state.Running {
		err := c.deleteInstance()
		if err != nil {
			log.Errorf("Error deleting instance: %v", err)
		}
	}
	return c.deleteDisk()
}

// Restart deletes and recreates the GCE instance, keeping the disk.
func (driver *Driver) Restart() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	if err := c.deleteInstance(); err != nil {
		return err
	}

	return c.createInstance(driver.publicSSHKeyPath, driver.sshKeyPath, driver.MachineType)
}

// Kill deletes the GCE instance, but keeps the disk.
func (driver *Driver) Kill() error {
	return driver.Stop()
}

// GetSSHCommand returns a command that will run over SSH on the GCE instance.
func (driver *Driver) GetSSHCommand(args ...string) (*exec.Cmd, error) {
	ip, _ := driver.GetIP()
	return ssh.GetSSHCommand(ip, 22, driver.UserName, driver.sshKeyPath, args...), nil
}

// Upgrade upgrades the docker daemon on the host to the latest version.
func (driver *Driver) Upgrade() error {
	c, err := newComputeUtil(driver)
	if err != nil {
		return err
	}
	ip, err := driver.GetIP()
	if err != nil {
		return err
	}
	return c.updateDocker(ip, driver.sshKeyPath)
}
