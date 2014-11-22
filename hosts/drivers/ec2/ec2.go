package ec2

import (
	"encoding/xml"
	"fmt"

	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/pkg/log"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"
	awsauth "github.com/smartystreets/go-aws-auth"
)

type Driver struct {
	Auth          Auth
	Endpoint      string
	ImageId       string
	InstanceId    string
	InstanceName  string
	InstanceType  string
	KeyName       string
	NoInstall     bool
	NoProvision   bool
	PublicDnsName string
	IPAddress     string
	SecurityGroup string
	Region        string
	Username      string
	storePath     string
}

type CreateFlags struct {
	AccessKey     *string
	SecretKey     *string
	ImageId       *string
	InstanceName  *string
	NoInstall     *bool
	NoProvision   *bool
	Region        *string
	Username      *string
	SecurityGroup *string
	InstanceType  *string
}

type Instance struct {
	info Ec2Instance
}

func init() {
	drivers.Register("ec2", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

const (
	// maximum number of attempts to add ingress rules to the security group
	MAX_REQUEST_ATTEMPTS        int = 5
	MAX_SSH_CONNECTION_ATTEMPTS int = 1000

	// "Ubuntu 14.04 LTS with Docker and Runit"
	DEFAULT_IMAGE_ID string = "ami-27939962"

	DEFAULT_INSTANCE_TYPE  string = "m1.small"
	DEFAULT_SSH_USERNAME   string = "ubuntu"
	DEFAULT_SECURITY_GROUP string = "docker-hosts"
	DEFAULT_REGION         string = "us-west-1"
)

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.AccessKey = cmd.String(
		[]string{"-ec2-access-key"},
		"",
		"AWS Access Key",
	)
	createFlags.SecretKey = cmd.String(
		[]string{"-ec2-secret-key"},
		"",
		"AWS Secret Key",
	)
	createFlags.ImageId = cmd.String(
		[]string{"-ec2-image-id"},
		DEFAULT_IMAGE_ID,
		"AMI to use for the selected region",
	)
	createFlags.InstanceName = cmd.String(
		[]string{"-ec2-instance-name"},
		"",
		"Name of created instance",
	)
	createFlags.Region = cmd.String(
		[]string{"-ec2-region"},
		DEFAULT_REGION,
		"AWS Region",
	)
	createFlags.InstanceType = cmd.String(
		[]string{"-ec2-instance-type"},
		DEFAULT_INSTANCE_TYPE,
		"Type of instance to create",
	)
	createFlags.NoInstall = cmd.Bool(
		[]string{"-no-install"},
		false,
		"Do not install Docker during provisioning, assume it already exists",
	)
	createFlags.NoProvision = cmd.Bool(
		[]string{"-no-provision"},
		false,
		"Do not provision the instance automatically",
	)
	createFlags.Username = cmd.String(
		[]string{"-ec2-instance-username"},
		DEFAULT_SSH_USERNAME,
		"Username for SSH on the instance (depends on AMI)",
	)

	// TODO: user should be able to specify multiple security groups
	// also, the default really should be that we create one automatically
	// and/or lookup to see if there's an existing one for our use case
	createFlags.SecurityGroup = cmd.String(
		[]string{"-ec2-security-group"},
		DEFAULT_SECURITY_GROUP,
		"Security group to use for the created instance",
	)
	return createFlags
}

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func (d *Driver) DriverName() string {
	return "ec2"
}

func (d *Driver) GetURL() (string, error) {
	if d.IPAddress == "" {
		return "", nil
	}
	return fmt.Sprintf("tcp://%s:2375", d.IPAddress), nil
}

func (d *Driver) GetIP() (string, error) {
	if d.IPAddress == "" {
		return "", fmt.Errorf("IP Address does not exist yet")
	}
	return d.IPAddress, nil
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	d.Auth = Auth{
		*flags.AccessKey,
		*flags.SecretKey,
	}
	d.ImageId = *flags.ImageId
	d.Region = *flags.Region
	d.InstanceType = *flags.InstanceType
	d.InstanceName = *flags.InstanceName
	d.Endpoint = fmt.Sprintf("https://ec2.%s.amazonaws.com", d.Region)
	d.Username = *flags.Username
	d.SecurityGroup = *flags.SecurityGroup
	d.NoInstall = *flags.NoInstall
	d.NoProvision = *flags.NoProvision

	if d.Auth.AccessKey == "" || d.Auth.SecretKey == "" {
		var err error
		d.Auth, err = EnvAuth()
		if err != nil {
			return fmt.Errorf("Error setting the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables :%s", err)
		}
	} else {
		d.Auth.AccessKey = *flags.AccessKey
		d.Auth.SecretKey = *flags.SecretKey
	}

	return nil
}

func (d *Driver) Create() error {
	d.setInstanceNameIfNotSet()
	if err := d.createSecurityGroup(); err != nil {
		return fmt.Errorf("Error creating security group: %s", err)
	}

	log.Infof("Creating key pair...")
	if err := d.createKeyPair(); err != nil {
		return fmt.Errorf("Error creating key pair: %s", err)
	}

	log.Infof("Creating AWS EC2 instance...")
	instance, err := d.runInstance()
	if err != nil {
		return fmt.Errorf("Error running the EC2 instance: %s", err)
	}

	d.InstanceId = instance.info.InstanceId

	if !d.NoProvision {
		if err := d.provision(); err != nil {
			return fmt.Errorf("Error provisioning instance: %s", err)
		}
	}

	log.Infof("Tagging instance %s", d.InstanceName)
	if err := d.tagInstance("Name", d.InstanceName); err != nil {
		return fmt.Errorf("Error tagging instance name: %s", err)
	}

	return nil
}

func (d *Driver) provision() error {
	log.Infof("Waiting for SSH to become available...")
	attempts := 0

	for {
		// wait for instance SSH to come up
		fmt.Print(".")
		time.Sleep(1 * time.Second)
		attempts++

		// check state so we can get instance IP address
		_, err := d.GetState()
		if err != nil {
			return fmt.Errorf("Error getting instance state: %s", err)
		}
		if d.IPAddress == "" {
			continue
		}

		if _, err := net.DialTimeout("tcp", fmt.Sprintf("%s:22", d.IPAddress), 1*time.Second); err != nil {
			if attempts < MAX_SSH_CONNECTION_ATTEMPTS {
				continue
			} else {
				return fmt.Errorf("SSH max attempts exceeded and last error was: %s", err)
			}
		}
		fmt.Println()
		break
	}

	log.Infof("Provisioning instance...")

	if !d.NoInstall {
		log.Infof("Downloading latest version of docker and setting up system service...")
		if err := d.GetSSHCommand("curl -sSL https://get.docker.com/ | sudo sh").Run(); err != nil {
			return fmt.Errorf("Error curl'ing and executing docker installation script: %s", err)
		}
	}

	log.Infof("Setting daemon options to allow connection over TCP...")
	if err := d.GetSSHCommand("echo 'DOCKER_OPTS=\"--host 0.0.0.0:2375\"' | sudo tee /etc/default/docker").Run(); err != nil {
		return fmt.Errorf("Error running command to add daemon options over SSH : %s", err)
	}

	log.Infof("Restarting docker service to reflect changes...")
	if err := d.GetSSHCommand("sudo service docker restart").Run(); err != nil {
		return fmt.Errorf("Error restarting docker service over SSH : %s", err)
	}

	return nil
}

func (d *Driver) tagInstance(key string, val string) error {
	v := url.Values{}
	v.Set("Action", "CreateTags")
	v.Set("ResourceId.1", d.InstanceId)
	v.Set("Tag.1.Key", key)
	v.Set("Tag.1.Value", val)
	if _, err := d.makeAwsApiCall(v); err != nil {
		return newAwsApiCallError(err)
	}
	return nil
}

func (d *Driver) deleteKeyPair() error {
	v := url.Values{}
	v.Set("Action", "DeleteKeyPair")
	v.Set("KeyName", d.KeyName)

	// Once again, TODO: actually use this response?
	_, err := d.makeAwsApiCall(v)
	if err != nil {
		return fmt.Errorf("Error making API call to delete keypair :%s", err)
	}
	return nil
}

func (d *Driver) createKeyPair() error {
	d.KeyName = fmt.Sprintf("%s-key", d.InstanceName)
	v := url.Values{}
	v.Set("Action", "CreateKeyPair")
	v.Set("KeyName", d.KeyName)
	resp, err := d.makeAwsApiCall(v)
	if err != nil {
		return fmt.Errorf("Error trying API call to create keypair: %s", err)
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading AWS response body")
	}

	unmarshalledResponse := CreateKeyPairResponse{}
	if xml.Unmarshal(contents, &unmarshalledResponse); err != nil {
		return fmt.Errorf("Error unmarshalling AWS response XML: %s", err)
	}

	key := unmarshalledResponse.KeyMaterial
	if err := ioutil.WriteFile(d.sshKeyPath(), key, 0400); err != nil {
		return fmt.Errorf("Error writing SSH key to file: %s", err)
	}

	return nil
}

func (d *Driver) setInstanceNameIfNotSet() {
	if d.InstanceName == "" {
		d.InstanceName = fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	}
}

func (d *Driver) createSecurityGroup() error {
	v := url.Values{}
	v.Set("Action", "CreateSecurityGroup")
	v.Set("GroupName", d.SecurityGroup)
	v.Set("GroupDescription", url.QueryEscape("default for instances created by docker hosts"))

	resp, err := d.makeAwsApiCall(v)
	defer resp.Body.Close()
	if err != nil {
		// ugly hack since API has no way to check if SG already exists
		if resp.StatusCode == http.StatusBadRequest {
			var errorResponse ErrorResponse
			if err := getDecodedResponse(resp, &errorResponse); err != nil {
				return fmt.Errorf("Error decoding error response: %s", err)
			}
			if errorResponse.Errors[0].Code == ErrorDuplicateGroup {
				log.Infof("Security group docker-hosts exists, using...")
				return nil
			}
		}
		return fmt.Errorf("Error making API call to create security group: %s", err)
	}

	log.Infof("Security group %s created", d.SecurityGroup)
	createSecurityGroupResponse := CreateSecurityGroupResponse{}

	if err := getDecodedResponse(resp, &createSecurityGroupResponse); err != nil {
		return fmt.Errorf("Error decoding create security groups response: %s", err)
	}

	// have a number of retries queued to manage eventual consistency issue
	for attempts := 0; attempts < MAX_REQUEST_ATTEMPTS; attempts++ {
		v := url.Values{}
		v.Set("Action", "AuthorizeSecurityGroupIngress")
		v.Set("GroupId", createSecurityGroupResponse.GroupId)
		ingressPortsAllowed := []string{
			"22",
			"80",
			"2375",
		}
		for index, port := range ingressPortsAllowed {
			n := index + 1 // amazon starts counting from 1 not 0
			v.Set(fmt.Sprintf("IpPermissions.%d.IpProtocol", n), "tcp")
			v.Set(fmt.Sprintf("IpPermissions.%d.FromPort", n), port)
			v.Set(fmt.Sprintf("IpPermissions.%d.ToPort", n), port)
			v.Set(fmt.Sprintf("IpPermissions.%d.IpRanges.1.CidrIp", n), "0.0.0.0/0")
		}
		resp, err := d.makeAwsApiCall(v)
		defer resp.Body.Close()
		if err != nil {
			if resp.StatusCode == http.StatusBadRequest {
				continue
			} else {
				return fmt.Errorf("Error making API call to authorize security group ingress: %s", err)
			}
		}
		if os.Getenv("DEBUG") != "" {
			contents, _ := ioutil.ReadAll(resp.Body)
			fmt.Println(string(contents))
		}
		time.Sleep(time.Second * 1)
		return nil
	}

	return nil
}

func getDecodedResponse(r http.Response, into interface{}) error {
	defer r.Body.Close()
	if err := xml.NewDecoder(r.Body).Decode(into); err != nil {
		return fmt.Errorf("Error decoding error response: %s", err)
	}
	return nil
}

func newAwsApiResponseError(r http.Response) error {
	return fmt.Errorf("Non-200 API response: %d", r.StatusCode)
}

func (d *Driver) makeAwsApiCall(v url.Values) (http.Response, error) {
	v.Set("Version", "2014-06-15")
	client := &http.Client{}
	finalEndpoint := fmt.Sprintf("%s?%s", d.Endpoint, v.Encode())
	req, err := http.NewRequest("GET", finalEndpoint, nil)
	if err != nil {
		return http.Response{}, fmt.Errorf("Error creating request from client")
	}
	req.Header.Add("Content-type", "application/json")
	awsauth.Sign(req, awsauth.Credentials{
		AccessKeyID:     d.Auth.AccessKey,
		SecretAccessKey: d.Auth.SecretKey,
	})
	resp, err := client.Do(req)
	if err != nil {
		return *resp, fmt.Errorf("Client encountered error while doing the request: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return *resp, newAwsApiResponseError(*resp)
	}
	return *resp, nil
}

func newAwsApiCallError(err error) error {
	return fmt.Errorf("Problem with AWS API call: %s", err)
}

func (d *Driver) runInstance() (Instance, error) {
	instance := Instance{}
	v := url.Values{}
	v.Set("Action", "RunInstances")
	v.Set("ImageId", d.ImageId)
	v.Set("Placement.AvailabilityZone", d.Region+"a")
	v.Set("SecurityGroup.1", d.SecurityGroup)
	v.Set("MinCount", "1")
	v.Set("MaxCount", "1")
	v.Set("KeyName", d.KeyName)
	resp, err := d.makeAwsApiCall(v)
	if err != nil {
		return instance, newAwsApiCallError(err)
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return instance, fmt.Errorf("Error reading AWS response body")
	}
	unmarshalledResponse := RunInstancesResponse{}
	err = xml.Unmarshal(contents, &unmarshalledResponse)
	if err != nil {
		return instance, fmt.Errorf("Error unmarshalling AWS response XML: %s", err)
	}

	instance.info = unmarshalledResponse.Instances[0]
	return instance, nil
}

func (d *Driver) performStandardAction(action string) (http.Response, error) {
	v := url.Values{}
	v.Set("Action", action)
	v.Set("InstanceId.1", d.InstanceId)
	resp, err := d.makeAwsApiCall(v)
	if err != nil {
		return resp, newAwsApiCallError(err)
	}
	return resp, nil
}

func (d *Driver) GetState() (state.State, error) {
	resp, err := d.performStandardAction("DescribeInstances")
	if err != nil {
		return state.Error, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return state.Error, fmt.Errorf("Error reading AWS response body: %s", err)
	}

	unmarshalledResponse := DescribeInstancesResponse{}
	if err = xml.Unmarshal(contents, &unmarshalledResponse); err != nil {
		return state.Error, fmt.Errorf("Error unmarshalling AWS response XML: %s", err)
	}

	reservationSet := unmarshalledResponse.ReservationSet[0]
	instanceState := reservationSet.InstancesSet[0].InstanceState
	networkInterfaceSet := reservationSet.InstancesSet[0].NetworkInterfaceSet

	association := networkInterfaceSet[0].Association
	d.IPAddress = association.PublicIp
	d.PublicDnsName = association.PublicDnsName

	shortState := strings.TrimSpace(instanceState.Name)
	switch shortState {
	case "pending":
		return state.Starting, nil
	case "running":
		return state.Running, nil
	case "stopped":
		return state.Stopped, nil
	case "stopping":
		return state.Stopped, nil
	}

	return state.Error, nil
}

// TODO: Do something useful with the following API responses
//       which are currently just getting discarded?
func (d *Driver) Remove() error {
	if err := d.deleteKeyPair(); err != nil {
		return err
	}
	if _, err := d.performStandardAction("TerminateInstances"); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Start() error {
	if _, err := d.performStandardAction("StartInstances"); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Stop() error {
	if _, err := d.performStandardAction("StopInstances"); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Restart() error {
	if _, err := d.performStandardAction("RebootInstances"); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Kill() error {
	// Not really anything like a hard power-off / kill
	// in the AWS API that I can find.  Perhaps I am wrong!
	if _, err := d.performStandardAction("StopInstances"); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Upgrade() error {
	// TODO: implement
	return nil
}

func (d *Driver) sshKeyPath() string {
	return path.Join(d.storePath, fmt.Sprintf("%s.pem", d.KeyName))
}

func (d *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	return ssh.GetSSHCommand(d.IPAddress, 22, d.Username, d.sshKeyPath(), args...)
}
