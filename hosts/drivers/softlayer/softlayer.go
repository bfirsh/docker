package softlayer

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"time"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/hosts/utils"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/log"
	flag "github.com/docker/docker/pkg/mflag"
)

const ApiEndpoint = "https://api.softlayer.com/rest/v3"
const DockerInstallUrl = "https://get.docker.com"

type Driver struct {
	storePath    string
	IPAddress    string
	deviceConfig *deviceConfig
	Id           int
	Client       *Client
}

type deviceConfig struct {
	DiskSize      int
	Cpu           int
	Hostname      string
	Domain        string
	Region        string
	Memory        int
	Image         string
	HourlyBilling bool
	InstallScript string
	LocalDisk     bool
	PrivateNet    bool
}

type CreateFlags struct {
	Memory        *int
	DiskSize      *int
	Region        *string
	Cpu           *int
	Hostname      *string
	Domain        *string
	Endpoint      *string
	Username      *string
	ApiKey        *string
	Id            *int
	HourlyBilling *bool
	LocalDisk     *bool
	Image         *string
	PrivateNet    *bool
	InstallScript *string
}

func init() {
	drivers.Register("softlayer", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.Memory = cmd.Int([]string{"-softlayer-memory"}, 1024, "Memory for host in MB")
	createFlags.DiskSize = cmd.Int([]string{"-softlayer-disk-size"}, 0, "Size of disk for host in MB")
	createFlags.Region = cmd.String([]string{"-softlayer-region"}, "dal05", "Region for host")
	createFlags.Cpu = cmd.Int([]string{"-softlayer-cpu"}, 1, "Number of CPU's for host")
	createFlags.Hostname = cmd.String([]string{"-softlayer-hostname"}, "docker", "Hostname for new host")
	createFlags.Domain = cmd.String([]string{"-softlayer-domain"}, "", "Doman name for new host")
	createFlags.Endpoint = cmd.String([]string{"-softlayer-api-endpoint"}, ApiEndpoint, "Set custom API endpoint")
	createFlags.Username = cmd.String([]string{"-softlayer-user"}, "", "Softlayer Username")
	createFlags.ApiKey = cmd.String([]string{"-softlayer-api-key"}, "", "Softlayer API key")
	createFlags.HourlyBilling = cmd.Bool([]string{"-softlayer-hourly-billing"}, true, "Use hourly billing")
	createFlags.LocalDisk = cmd.Bool([]string{"-softlayer-local-disk"}, false, "Use local disk instead of SAN")
	createFlags.PrivateNet = cmd.Bool([]string{"-softlayer-private-net-only"}, false, "Do not use public network")
	createFlags.Image = cmd.String([]string{"-softlayer-image"}, "UBUNTU_LATEST", "Operating system image to use")
	createFlags.InstallScript = cmd.String([]string{"-softlayer-install-script"}, DockerInstallUrl, "Install script to call after VM is initialized (should install Docker)")
	return createFlags
}

func validateFlags(flags *CreateFlags) error {
	if *flags.Hostname == "" {
		return fmt.Errorf("Missing required setting - hostname")
	}
	if *flags.Domain == "" {
		return fmt.Errorf("Missing required setting - domain")
	}
	if *flags.Username == "" {
		return fmt.Errorf("Missing required setting - user")
	}
	if *flags.ApiKey == "" {
		return fmt.Errorf("Missing required setting - API key")
	}
	if *flags.Region == "" {
		return fmt.Errorf("Missing required setting - Region")
	}
	if *flags.Cpu < 1 {
		return fmt.Errorf("Missing required setting - Cpu")
	}

	return nil
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	if err := validateFlags(flags); err != nil {
		return err
	}

	d.Client = &Client{
		Endpoint: *flags.Endpoint,
		User:     *flags.Username,
		ApiKey:   *flags.ApiKey,
	}

	d.deviceConfig = &deviceConfig{
		Hostname:      *flags.Hostname,
		DiskSize:      *flags.DiskSize,
		Cpu:           *flags.Cpu,
		Domain:        *flags.Domain,
		Memory:        *flags.Memory,
		PrivateNet:    *flags.PrivateNet,
		LocalDisk:     *flags.LocalDisk,
		HourlyBilling: *flags.HourlyBilling,
		InstallScript: *flags.InstallScript,
		Image:         "UBUNTU_LATEST",
		Region:        *flags.Region,
	}

	return nil
}

func (d *Driver) getClient() *Client {
	return d.Client
}

func (d *Driver) DriverName() string {
	return "softlayer"
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	if ip == "" {
		return "", nil
	}
	return "tcp://" + ip + ":2376", nil
}

func (d *Driver) GetIP() (string, error) {
	if d.IPAddress != "" {
		return d.IPAddress, nil
	}
	return d.getClient().VirtualGuest().GetPublicIp(d.Id)
}

func (d *Driver) GetState() (state.State, error) {
	s, err := d.getClient().VirtualGuest().PowerState(d.Id)
	if err != nil {
		return state.None, err
	}
	var vmState state.State
	switch s {
	case "Running":
		vmState = state.Running
	case "Halted":
		vmState = state.Stopped
	default:
		vmState = state.None
	}
	return vmState, nil
}

func (d *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	return ssh.GetSSHCommand(d.IPAddress, 22, "root", d.sshKeyPath(), args...)
}

func (d *Driver) Create() error {
	waitForStart := func() {
		fmt.Printf("Waiting for host to become available")
		for {
			s, err := d.GetState()
			if err != nil {
				continue
			}

			if s == state.Running {
				break
			}
			fmt.Printf(".")
			time.Sleep(2 * time.Second)
		}
		fmt.Printf("\n")
	}

	getIp := func() {
		fmt.Printf("Getting Host IP")
		for {
			var (
				ip  string
				err error
			)
			if d.deviceConfig.PrivateNet {
				ip, err = d.getClient().VirtualGuest().GetPrivateIp(d.Id)
			} else {
				ip, err = d.getClient().VirtualGuest().GetPublicIp(d.Id)
			}
			if err != nil {
				time.Sleep(2 * time.Second)
				fmt.Printf(".")
				continue
			}
			// not a perfect regex, but should be just fine for our needs
			exp := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
			if exp.MatchString(ip) {
				d.IPAddress = ip
				break
			}
			time.Sleep(2 * time.Second)
			fmt.Printf(".")
		}
		fmt.Printf("\n")
	}

	log.Infof("Creating SSH key...")
	key, err := d.createSSHKey()
	if err != nil {
		return err
	}

	spec := d.buildHostSpec()
	spec.SshKeys = []*SshKey{key}

	id, err := d.getClient().VirtualGuest().Create(spec)
	if err != nil {
		return fmt.Errorf("Error creating host: %q", err)
	}
	d.Id = id
	getIp()
	waitForStart()
	if err := d.setupHost(); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up host config: %q", err)
	}
	return nil
}

func (d *Driver) buildHostSpec() *HostSpec {
	spec := &HostSpec{
		Hostname:       d.deviceConfig.Hostname,
		Domain:         d.deviceConfig.Domain,
		Cpu:            d.deviceConfig.Cpu,
		Memory:         d.deviceConfig.Memory,
		Datacenter:     Datacenter{Name: d.deviceConfig.Region},
		InstallScript:  d.deviceConfig.InstallScript,
		Os:             d.deviceConfig.Image,
		HourlyBilling:  d.deviceConfig.HourlyBilling,
		PrivateNetOnly: d.deviceConfig.PrivateNet,
	}
	if d.deviceConfig.DiskSize > 0 {
		spec.BlockDevices = []BlockDevice{BlockDevice{Device: "0", DiskImage: DiskImage{Capacity: d.deviceConfig.DiskSize}}}
	}
	return spec
}

func (d *Driver) createSSHKey() (*SshKey, error) {
	if err := ssh.GenerateSSHKey(d.sshKeyPath()); err != nil {
		return nil, err
	}

	publicKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
	if err != nil {
		return nil, err
	}

	key, err := d.getClient().SshKey().Create(d.deviceConfig.Hostname, string(publicKey))
	if err != nil {
		return nil, err
	}

	return key, nil
}

func (d *Driver) publicSSHKeyPath() string {
	return d.sshKeyPath() + ".pub"
}

func (d *Driver) sshKeyPath() string {
	return path.Join(d.storePath, "id_rsa")
}

func (d *Driver) Kill() error {
	return nil
}
func (d *Driver) Remove() error {
	var err error
	for i := 0; i < 5; i++ {
		if err = d.getClient().VirtualGuest().Cancel(d.Id); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}
	return err
}
func (d *Driver) Restart() error {
	return d.getClient().VirtualGuest().Reboot(d.Id)
}
func (d *Driver) Start() error {
	return d.getClient().VirtualGuest().PowerOn(d.Id)
}
func (d *Driver) Stop() error {
	return d.getClient().VirtualGuest().PowerOff(d.Id)
}

func (d *Driver) setupHost() error {
	fmt.Printf("Configuring host")
	if err := d.createTlsCerts(); err != nil {
		return err
	}
	a, err := archive.TarWithOptions(path.Dir(d.storePath+"/certs"), &archive.TarOptions{
		Compression: archive.Uncompressed,
		Name:        ".docker",
		Includes:    []string{path.Base(d.storePath + "/certs")},
	})
	b, err := ioutil.ReadAll(a)
	if err != nil {
		return err
	}
	ssh.WaitForTCP(d.IPAddress + ":22")
	cmd := d.GetSSHCommand(fmt.Sprintf("echo -n -e %q > /root/certs.tar && cd /root && tar -xvf certs.tar && rm certs.tar", string(b)))
	if err := cmd.Run(); err != nil {
		return err
	}

	for {
		cmd = d.GetSSHCommand(`[ -f "$(which docker)" ] && [ -f "/etc/default/docker" ] || exit 1`)
		if err := cmd.Run(); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
		fmt.Printf(".")
	}

	cmd = d.GetSSHCommand(`echo 'DOCKER_OPTS="--tlsverify --tls --tlscacert /root/.docker/ca.pem --tlscert /root/.docker/srv_cert.pem --tlskey /root/.docker/srv_key.pem -H tcp://0.0.0.0:2376 -H unix:///var/run/docker.sock"' >> /etc/default/docker && stop docker && start docker`)
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Printf("\n")
	return nil
}

func (d *Driver) createTlsCerts() error {
	os.MkdirAll(d.storePath+"/certs", 0755)
	caFile := d.storePath + "/certs/ca.pem"
	caKeyFile := d.storePath + "/certs/ca_key.pem"
	keyFile := d.storePath + "/certs/key.pem"
	certFile := d.storePath + "/certs/cert.pem"
	srvCertFile := d.storePath + "/certs/srv_cert.pem"
	srvKeyFile := d.storePath + "/certs/srv_key.pem"

	if err := utils.GenerateCA(caFile, caKeyFile); err != nil {
		return err
	}

	if err := utils.GenerateCert([]string{d.IPAddress}, srvCertFile, srvKeyFile, caFile, caKeyFile); err != nil {
		return err
	}

	return utils.GenerateCert([]string{""}, certFile, keyFile, caFile, caKeyFile)
}
