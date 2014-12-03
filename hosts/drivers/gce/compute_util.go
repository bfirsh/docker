package gce

import (
	"fmt"
	"io/ioutil"
	"time"

	raw "code.google.com/p/google-api-go-client/compute/v1"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/hosts/ssh"
)

// ComputeUtil is used to wrap the raw GCE API code and store common parameters.
type ComputeUtil struct {
	zone         string
	instanceName string
	userName     string
	project      string
	service      *raw.Service
	zoneURL      string
	globalURL    string
}

const (
	apiURL    = "https://www.googleapis.com/compute/v1/projects/"
	imageName = "https://www.googleapis.com/compute/v1/projects/google-containers/global/images/container-vm-v20141016"
)

// NewComputeUtil creates and initializes a ComputeUtil.
func newComputeUtil(driver *Driver) (*ComputeUtil, error) {
	service, err := newGCEService(driver.storePath)
	if err != nil {
		return nil, err
	}
	c := ComputeUtil{
		zone:         driver.Zone,
		instanceName: driver.InstanceName,
		userName:     driver.UserName,
		project:      driver.Project,
		service:      service,
		zoneURL:      apiURL + driver.Project + "/zones/" + driver.Zone,
		globalURL:    apiURL + driver.Project + "/global",
	}
	return &c, nil
}

func (c *ComputeUtil) diskName() string {
	return c.instanceName + "-disk"
}

// disk returns the gce Disk.
func (c *ComputeUtil) disk() (*raw.Disk, error) {
	return c.service.Disks.Get(c.project, c.zone, c.diskName()).Do()
}

// createDisk creates a persistent disk.
func (c *ComputeUtil) createDisk() error {
	log.Infof("Creating disk")
	op, err := c.service.Disks.Insert(c.project, c.zone, &raw.Disk{
		Name: c.diskName(),
	}).SourceImage(imageName).Do()
	if err != nil {
		return err
	}
	log.Infof("Waiting for disk...")
	return c.waitForOp(op.Name)
}

// deleteDisk deletes the persistent disk.
func (c *ComputeUtil) deleteDisk() error {
	log.Infof("Deleting disk.")
	op, err := c.service.Disks.Delete(c.project, c.zone, c.diskName()).Do()
	if err != nil {
		return err
	}
	log.Infof("Waiting for disk to delete.")
	return c.waitForOp(op.Name)
}

// instance retrieves the instance.
func (c *ComputeUtil) instance() (*raw.Instance, error) {
	return c.service.Instances.Get(c.project, c.zone, c.instanceName).Do()
}

// createInstance creates a GCE VM instance.
func (c *ComputeUtil) createInstance(publicSSHKeyPath, sshKeyPath, machineType string) error {
	log.Infof("Creating instance.")
	disk, err := c.disk()
	if disk == nil {
		if err := c.createDisk(); err != nil {
			return err
		}
	}
	op, err := c.service.Instances.Insert(c.project, c.zone, &raw.Instance{
		Name:        c.instanceName,
		Description: "docker host vm",
		MachineType: c.zoneURL + "/machineTypes/" + machineType,
		Disks: []*raw.AttachedDisk{
			{
				Boot:       true,
				AutoDelete: false,
				Type:       "PERSISTENT",
				Mode:       "READ_WRITE",
				Source:     c.zoneURL + "/disks/" + c.instanceName + "-disk",
			},
		},
		NetworkInterfaces: []*raw.NetworkInterface{
			{
				AccessConfigs: []*raw.AccessConfig{
					&raw.AccessConfig{Type: "ONE_TO_ONE_NAT"},
				},
				Network: c.globalURL + "/networks/default",
			},
		},
	}).Do()

	if err != nil {
		return err
	}
	log.Infof("Waiting for Instance...")
	if err = c.waitForOp(op.Name); err != nil {
		return err
	}

	instance, err := c.instance()
	if err != nil {
		return err
	}
	ip := instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
	c.waitForSSH(ip)

	// Update the SSH Key
	sshKey, err := ioutil.ReadFile(publicSSHKeyPath)
	if err != nil {
		return err
	}
	log.Infof("Uploading SSH Key")
	op, err = c.service.Instances.SetMetadata(c.project, c.zone, c.instanceName, &raw.Metadata{
		Fingerprint: instance.Metadata.Fingerprint,
		Items: []*raw.MetadataItems{
			{
				Key:   "sshKeys",
				Value: c.userName + ":" + string(sshKey) + "\n",
			},
		},
	}).Do()
	if err != nil {
		return err
	}
	log.Infof("Waiting for SSH Key")
	err = c.waitForOp(op.Name)
	if err != nil {
		return err
	}

	if err := c.configureInstance(ip, sshKeyPath); err != nil {
		return err
	}

	// Configure Docker
	return c.updateDocker(ip, sshKeyPath)
}

// deleteInstance deletes the instance, leaving the persistent disk.
func (c *ComputeUtil) deleteInstance() error {
	log.Infof("Deleting instance.")
	op, err := c.service.Instances.Delete(c.project, c.zone, c.instanceName).Do()
	if err != nil {
		return err
	}
	log.Infof("Waiting for instance to delete.")
	return c.waitForOp(op.Name)
}

// configureInstance prepares the instance for docker usage.
func (c *ComputeUtil) configureInstance(ip, sshKeyPath string) error {
	log.Infof("Setting up instance.")
	commands := []string{
		"sudo sed -i 's/DOCKER_OPTS=.*/DOCKER_OPTS=\"-H 0.0.0.0:2375\"/g' /etc/default/docker",
		"sudo service docker restart"}
	return c.executeCommands(commands, ip, sshKeyPath)
}

// updateDocker updates the docker daemon to the latest version.
func (c *ComputeUtil) updateDocker(ip, sshKeyPath string) error {
	log.Infof("Updating docker.")
	commands := []string{
		"sudo service docker stop",
		"sleep 10",
		"sudo wget https://get.docker.com/builds/Linux/x86_64/docker-latest -O /usr/bin/docker && sudo chmod +x /usr/bin/docker",
		"sudo service docker start"}
	return c.executeCommands(commands, ip, sshKeyPath)
}

func (c *ComputeUtil) executeCommands(commands []string, ip, sshKeyPath string) error {
	for _, command := range commands {
		log.Debugf("Running command: %v", command)
		cmd := ssh.GetSSHCommand(ip, 22, c.userName, sshKeyPath, command)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// waitForOp waits for the GCE Operation to finish.
func (c *ComputeUtil) waitForOp(name string) error {
	// Wait for the op to finish
	for {
		op, err := c.service.ZoneOperations.Get(c.project, c.zone, name).Do()
		if err != nil {
			return err
		}
		log.Debugf("operation %q status: %s", op.Name, op.Status)
		if op.Status == "DONE" {
			if op.Error != nil {
				return fmt.Errorf("Operation error: %v", *op.Error.Errors[0])
			}
			break
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

// waitForSSH waits for SSH to become ready on the instance.
func (c *ComputeUtil) waitForSSH(ip string) error {
	log.Infof("Waiting for SSH...")
	return ssh.WaitForTCP(fmt.Sprintf("%s:22", ip))
}

// ip retrieves and returns the external IP address of the instance.
func (c *ComputeUtil) ip() (string, error) {
	instance, err := c.service.Instances.Get(c.project, c.zone, c.instanceName).Do()
	if err != nil {
		return "", err
	}
	return instance.NetworkInterfaces[0].AccessConfigs[0].NatIP, nil
}
