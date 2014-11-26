/*
 * Copyright 2014 VMware, Inc.  All rights reserved.  Licensed under the Apache v2 License.
 */

package fusion

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/pkg/log"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"
)

// Driver for VMware Fusion
type Driver struct {
	Name           string
	Memory         int
	DiskSize       int
	ISO            string
	Boot2DockerURL string

	storePath string
}

type CreateFlags struct {
	Name           *string
	Boot2DockerURL *string
	Memory         *int
	DiskSize       *int
}

func init() {
	drivers.Register("fusion", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.Boot2DockerURL = cmd.String([]string{"-fusion-boot2docker-url"}, "", "Fusion URL for boot2docker image")
	createFlags.Memory = cmd.Int([]string{"-fusion-memory"}, 1024, "Fusion size of memory for host VM (in MB)")
	createFlags.DiskSize = cmd.Int([]string{"-fusion-disk-size"}, 20000, "Fusion size of disk for host VM (in MB)")
	return createFlags
}

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func (d *Driver) DriverName() string {
	return "fusion"
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	d.Memory = *flags.Memory
	d.DiskSize = *flags.DiskSize

	d.Boot2DockerURL = *flags.Boot2DockerURL
	// Autogenerate VM Name
	d.Name = generateVMName()

	d.ISO = path.Join(d.storePath, "boot2docker.iso")

	return nil
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	if ip == "" {
		return "", nil
	}
	return fmt.Sprintf("tcp://%s:2375", ip), nil
}

func (d *Driver) GetIP() (string, error) {
	ip, err := d.getIPfromDHCPLease()
	if err != nil {
		return "", err
	}

	return ip, nil
}

func (d *Driver) GetState() (state.State, error) {
	// VMRUN only tells use if the vm is running or not
	if stdout, _, _ := vmrun("list"); strings.Contains(stdout, d.vmxPath()) {
		return state.Running, nil
	}
	return state.Stopped, nil
}

func (d *Driver) Create() error {
	log.Infof("Downloading boot2docker...")
	if err := d.downloadISO(); err != nil {
		return err
	}

	log.Infof("Creating SSH key...")
	if err := ssh.GenerateSSHKey(d.sshKeyPath()); err != nil {
		return err
	}

	log.Infof("Creating VM...")
	if err := os.MkdirAll(d.storePath, 0755); err != nil {
		return err
	}

	if _, err := os.Stat(d.vmxPath()); err == nil {
		return ErrMachineExist
	}

	// Generate vmx config file from template
	vmxt := template.Must(template.New("vmx").Parse(vmx))
	vmxfile, err := os.Create(d.vmxPath())
	if err != nil {
		return err
	}
	vmxt.Execute(vmxfile, d)

	// Generate vmdk file
	diskImg := filepath.Join(d.storePath, fmt.Sprintf("%s.vmdk", d.Name))
	if _, err := os.Stat(diskImg); err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		if err := vdiskmanager(diskImg, d.DiskSize); err != nil {
			return err
		}
	}

	if err := d.Start(); err != nil {
		return err
	}

	var ip string

	log.Infof("Waiting for VM to come online...")
	for i := 1; i <= 60; i++ {
		ip, err = d.getIPfromDHCPLease()
		if err != nil {
			log.Debugf("Not yet there, error: %s", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if ip != "" {
			log.Debugf("Got an ip: %s", ip)
			break
		}
	}

	if ip != "" {
		return nil
	}

	return fmt.Errorf("Machine didn't return an IP after 120 seconds, aborting")
}

func (d *Driver) Start() error {
	vmrun("start", d.vmxPath(), "nogui")
	return nil
}

func (d *Driver) Stop() error {
	vmrun("stop", d.vmxPath(), "nogui")
	return nil
}

func (d *Driver) Remove() error {

	s, _ := d.GetState()
	if s == state.Running {
		if err := d.Kill(); err != nil {
			return fmt.Errorf("Error stopping VM before deletion")
		}
	}

	vmrun("deleteVM", d.vmxPath(), "nogui")
	return nil
}

func (d *Driver) Restart() error {
	vmrun("reset", d.vmxPath(), "nogui")
	return nil
}

func (d *Driver) Kill() error {
	vmrun("stop", d.vmxPath(), "nogui")
	return nil
}

func (d *Driver) Upgrade() error {
	return nil
}

func (d *Driver) GetSSHCommand(args ...string) (*exec.Cmd, error) {

	ip, err := d.GetIP()
	if err != nil {
		return nil, err
	}
	return ssh.GetSSHCommand(ip, 22, "root", d.sshKeyPath(), args...), nil
}

func (d *Driver) sshKeyPath() string {
	return filepath.Join(d.storePath, "id_docker_host_fusion")
}

func (d *Driver) publicSSHKeyPath() string {
	return d.sshKeyPath() + ".pub"
}

func (d *Driver) vmxPath() string {
	return path.Join(d.storePath, fmt.Sprintf("%s.vmx", d.Name))
}

func (d *Driver) vmdkPath() string {
	return path.Join(d.storePath, fmt.Sprintf("%s.vmdk", d.Name))
}

func (d *Driver) downloadISO() error {

	var isourl string
	if d.Boot2DockerURL == "" {
		// HACK: downloading b2d iso with VMware tools and DOCKER_TLS=no.
		isourl = "http://downloads.gosddc.io/boot2docker/boot2docker.iso"
	} else {
		isourl = d.Boot2DockerURL
	}

	rsp, err := http.Get(isourl)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	// Download to a temp file first then rename it to avoid partial download.
	f, err := ioutil.TempFile(d.storePath, "b2d.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, rsp.Body); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), d.ISO); err != nil {
		return err
	}
	return nil
}

func generateVMName() string {
	randomID := utils.TruncateID(utils.GenerateRandomID())
	return fmt.Sprintf("docker-host-%s", randomID)
}

func (d *Driver) getIPfromDHCPLease() (string, error) {

	var vmxfh *os.File
	var dhcpfh *os.File
	var vmxcontent []byte
	var dhcpcontent []byte
	var macaddr string
	var err error
	var lastipmatch string
	var currentip string
	var lastleaseendtime time.Time
	var currentleadeendtime time.Time

	// DHCP lease table for NAT vmnet interface
	var dhcpfile = "/var/db/vmware/vmnet-dhcpd-vmnet8.leases"

	if vmxfh, err = os.Open(d.vmxPath()); err != nil {
		return "", err
	}
	defer vmxfh.Close()

	if vmxcontent, err = ioutil.ReadAll(vmxfh); err != nil {
		return "", err
	}

	// Look for generatedAddress as we're passing a VMX with addressType = "generated".
	vmxparse := regexp.MustCompile(`^ethernet0.generatedAddress\s*=\s*"(.*?)"\s*$`)
	for _, line := range strings.Split(string(vmxcontent), "\n") {
		if matches := vmxparse.FindStringSubmatch(line); matches == nil {
			continue
		} else {
			macaddr = strings.ToLower(matches[1])
		}
	}

	if macaddr == "" {
		return "", fmt.Errorf("couldn't find MAC address in VMX file %s", d.vmxPath())
	}

	log.Debugf("MAC address in VMX: %s", macaddr)
	if dhcpfh, err = os.Open(dhcpfile); err != nil {
		return "", err
	}
	defer dhcpfh.Close()

	if dhcpcontent, err = ioutil.ReadAll(dhcpfh); err != nil {
		return "", err
	}

	// Get the IP from the lease table.
	leaseip := regexp.MustCompile(`^lease (.+?) {$`)
	// Get the lease end date time.
	leaseend := regexp.MustCompile(`^\s*ends \d (.+?);$`)
	// Get the MAC address associated.
	leasemac := regexp.MustCompile(`^\s*hardware ethernet (.+?);$`)

	for _, line := range strings.Split(string(dhcpcontent), "\n") {

		if matches := leaseip.FindStringSubmatch(line); matches != nil {
			lastipmatch = matches[1]
			continue
		}

		if matches := leaseend.FindStringSubmatch(line); matches != nil {
			lastleaseendtime, _ = time.Parse("2006/01/02 15:04:05", matches[1])
			continue
		}

		if matches := leasemac.FindStringSubmatch(line); matches != nil && matches[1] == macaddr && currentleadeendtime.Before(lastleaseendtime) {
			currentip = lastipmatch
			currentleadeendtime = lastleaseendtime
		}
	}

	if currentip == "" {
		return "", fmt.Errorf("IP not found for MAC %s in DHCP leases", macaddr)
	}

	log.Debugf("IP found in DHCP lease table: %s", currentip)
	return currentip, nil

}
