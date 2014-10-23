package rackspace

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"strings"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/pkg/log"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"

	"github.com/rackspace/gophercloud"
	osdisk "github.com/rackspace/gophercloud/openstack/compute/v2/extensions/diskconfig"
	oskey "github.com/rackspace/gophercloud/openstack/compute/v2/extensions/keypairs"
	osservers "github.com/rackspace/gophercloud/openstack/compute/v2/servers"
	"github.com/rackspace/gophercloud/rackspace"
	"github.com/rackspace/gophercloud/rackspace/compute/v2/keypairs"
	"github.com/rackspace/gophercloud/rackspace/compute/v2/servers"
)

type Driver struct {
	Username    string
	APIKey      string
	Region      string
	ImageID     string
	FlavorID    string
	KeyPairName string

	storePath    string
	Basename     string
	ServerID     string
	ServerIPAddr string
}

type CreateFlags struct {
	Username *string
	APIKey   *string
	Region   *string
	ImageID  *string
	FlavorID *string
}

func init() {
	drivers.Register("rackspace", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

func errMissingOption(flagName string) error {
	return fmt.Errorf("rackspace driver requires the --rackspace-%s option", flagName)
}

func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	return &CreateFlags{
		Username: cmd.String(
			[]string{"-rackspace-username"},
			"",
			"Rackspace account username",
		),
		APIKey: cmd.String(
			[]string{"-rackspace-api-key"},
			"",
			"Rackspace API key",
		),
		Region: cmd.String(
			[]string{"-rackspace-region"},
			"",
			"Rackspace region",
		),
		ImageID: cmd.String(
			[]string{"-rackspace-image"},
			"",
			"Rackspace image ID",
		),
		FlavorID: cmd.String(
			[]string{"-rackspace-flavor"},
			"",
			"Rackspace flavor ID",
		),
	}
}

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func (d *Driver) DriverName() string {
	return "rackspace"
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	d.Username = *flags.Username
	d.APIKey = *flags.APIKey
	d.Region = *flags.Region
	d.ImageID = *flags.ImageID
	d.FlavorID = *flags.FlavorID

	if d.Username == "" {
		return errMissingOption("username")
	}
	if d.APIKey == "" {
		return errMissingOption("api-key")
	}
	if d.Region == "" {
		return errMissingOption("region")
	}
	if d.ImageID == "" {
		return errMissingOption("image")
	}
	if d.FlavorID == "" {
		return errMissingOption("flavor")
	}

	return nil
}

func (d *Driver) Create() error {
	d.Basename = utils.GenerateRandomID()

	log.Infof("Creating Rackspace server...")

	client, err := d.authenticate()
	if err != nil {
		return err
	}

	if err := d.createSSHKey(client); err != nil {
		return err
	}

	if err := d.createServer(client); err != nil {
		return err
	}

	if err := d.setupDocker(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) GetIP() (string, error) {
	if d.ServerIPAddr == "" {
		return "", errors.New("Server has not been created yet.")
	}
	return d.ServerIPAddr, nil
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tcp://%s:2375", ip), nil
}

func (d *Driver) GetState() (state.State, error) {
	client, err := d.authenticate()
	if err != nil {
		return state.None, err
	}

	current, err := servers.Get(client, d.ServerID).Extract()
	if err != nil {
		return state.None, err
	}

	switch current.Status {
	case "BUILD":
		return state.Starting, nil
	case "ACTIVE":
		return state.Running, nil
	case "SUSPENDED":
		return state.Paused, nil
	case "DELETED":
		return state.Stopped, nil
	}

	return state.None, nil
}

func (d *Driver) Start() error {
	return errors.New("Unsupported at this time.")
}

func (d *Driver) Stop() error {
	return errors.New("Unsupported at this time.")
}

func (d *Driver) Remove() error {
	client, err := d.authenticate()
	if err != nil {
		return err
	}

	log.Debugf("Deleting this server.")

	if err := servers.Delete(client, d.ServerID); err != nil {
		return err
	}

	log.Debugf("Deleting the ssh keypair.")
	if err := keypairs.Delete(client, d.KeyPairName).Extract(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Restart() error {
	client, err := d.authenticate()
	if err != nil {
		return err
	}

	log.Debugf("Restarting the server.")

	if err := servers.Reboot(client, d.ServerID, osservers.SoftReboot).Extract(); err != nil {
		return err
	}

	log.Debugf("Waiting for server to reboot.")
	if err := servers.WaitForStatus(client, d.ServerID, "ACTIVE", 600); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Kill() error {
	return d.Remove()
}

func (d *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	return ssh.GetSSHCommand(d.ServerIPAddr, 22, "core", d.sshKeyPath(), args...)
}

func (d *Driver) authenticate() (*gophercloud.ServiceClient, error) {
	log.Debugf("Authenticating with your Rackspace credentials.")

	ao := gophercloud.AuthOptions{
		Username: d.Username,
		APIKey:   d.APIKey,
	}

	providerClient, err := rackspace.AuthenticatedClient(ao)
	if err != nil {
		return nil, err
	}

	serviceClient, err := rackspace.NewComputeV2(providerClient, gophercloud.EndpointOpts{
		Region: d.Region,
	})
	if err != nil {
		return nil, err
	}

	return serviceClient, nil
}

func (d *Driver) createSSHKey(client *gophercloud.ServiceClient) error {
	log.Debugf("Creating a new SSH key.")

	if err := ssh.GenerateSSHKey(d.sshKeyPath()); err != nil {
		return err
	}

	publicKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
	if err != nil {
		return err
	}

	k, err := keypairs.Create(client, oskey.CreateOpts{
		Name:      "docker-key-" + d.Basename,
		PublicKey: string(publicKey),
	}).Extract()
	if err != nil {
		return err
	}

	d.KeyPairName = k.Name

	return nil
}

func (d *Driver) createServer(client *gophercloud.ServiceClient) error {
	log.Debugf("Launching the server.")
	name := "docker-host-" + d.Basename

	s, err := servers.Create(client, servers.CreateOpts{
		Name:       name,
		ImageRef:   d.ImageID,
		FlavorRef:  d.FlavorID,
		KeyPair:    d.KeyPairName,
		DiskConfig: osdisk.Manual,
	}).Extract()
	if err != nil {
		return err
	}

	log.Debugf("Waiting for server %s to launch.", name)
	if err = servers.WaitForStatus(client, s.ID, "ACTIVE", 300); err != nil {
		return err
	}

	log.Debugf("Getting details for server %s.", name)
	details, err := servers.Get(client, s.ID).Extract()
	if err != nil {
		return err
	}
	d.ServerID = details.ID
	d.ServerIPAddr = details.AccessIPv4

	log.Debugf("Server %s is ready at IP address %s.", name, d.ServerIPAddr)

	return nil
}

func (d *Driver) setupDocker() error {
	log.Debugf("Setting up Docker.")

	const init = `[Unit]
Description=Docker Socket for the API

[Socket]
ListenStream=2375
BindIPv6Only=both
Service=docker.service

[Install]
WantedBy=sockets.target`

	serviceCmds := []string{
		`systemctl enable docker-tcp.socket`,
		`systemctl stop docker`,
		`systemctl start docker-tcp.socket`,
		`systemctl start docker`,
	}

	commands := []string{
		fmt.Sprintf(`sudo sh -c "echo '%s' > /etc/systemd/system/docker-tcp.socket"`, init),
		fmt.Sprintf(`sudo sh -c "%s"`, strings.Join(serviceCmds, " && ")),
	}

	for _, command := range commands {
		log.Debugf("> %s", command)
		if err := d.GetSSHCommand(command).Run(); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) sshKeyPath() string {
	return path.Join(d.storePath, "id_rsa")
}

func (d *Driver) publicSSHKeyPath() string {
	return d.sshKeyPath() + ".pub"
}
