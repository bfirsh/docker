package azure

import (
	"fmt"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	azure "github.com/MSOpenTech/azure-sdk-for-go"
	"github.com/MSOpenTech/azure-sdk-for-go/clients/vmClient"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"
)

type Driver struct {
	SubscriptionID          string
	SubscriptionCert        string
	PublishSettingsFilePath string
	IPAddress               string
	Name                    string
	Location                string
	Size                    string
	UserName                string
	UserPassword            string
	Image                   string
	SshPort                 int
	DockerCertDir           string
	DockerPort              int
	storePath               string
}

type CreateFlags struct {
	SubscriptionID          *string
	SubscriptionCert        *string
	PublishSettingsFilePath *string
	Name                    *string
	Location                *string
	Size                    *string
	UserName                *string
	UserPassword            *string
	Image                   *string
	SshPort                 *string
	DockerCertDir           *string
	DockerPort              *string
}

func init() {
	drivers.Register("azure", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

//Region public methods starts

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.SubscriptionID = cmd.String(
		[]string{"-azure-subscription-id"},
		"",
		"Azure subscription ID",
	)
	createFlags.SubscriptionCert = cmd.String(
		[]string{"-azure-subscription-cert"},
		"",
		"Azure subscription cert",
	)
	createFlags.PublishSettingsFilePath = cmd.String(
		[]string{"-azure-publish-settings-file"},
		"",
		"Azure publish settings file",
	)
	createFlags.Location = cmd.String(
		[]string{"-azure-location"},
		"West US",
		"Azure location",
	)
	createFlags.Size = cmd.String(
		[]string{"-azure-size"},
		"Small",
		"Azure size",
	)
	createFlags.Name = cmd.String(
		[]string{"-azure-name"},
		"",
		"Azure name",
	)
	createFlags.UserName = cmd.String(
		[]string{"-azure-username"},
		"tcuser",
		"Azure username",
	)
	createFlags.UserPassword = cmd.String(
		[]string{"-azure-password"},
		"",
		"Azure user password",
	)
	createFlags.Image = cmd.String(
		[]string{"-azure-image"},
		"b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-14_04-LTS-amd64-server-20140724-en-us-30GB",
		"Azure image name",
	)
	createFlags.SshPort = cmd.String(
		[]string{"-azure-ssh"},
		"22",
		"Azure ssh port",
	)
	createFlags.DockerCertDir = cmd.String(
		[]string{"-azure-docker-cert-dir"},
		".docker",
		"Azure docker cert directory",
	)
	createFlags.DockerPort = cmd.String(
		[]string{"-azure-docker-port"},
		"4243",
		"Azure docker port",
	)
	return createFlags
}

func NewDriver(storePath string) (drivers.Driver, error) {
	driver := &Driver{storePath: storePath}
	return driver, nil
}

func (d *Driver) DriverName() string {
	return "azure"
}

func (driver *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	driver.SubscriptionID = *flags.SubscriptionID
	driver.SubscriptionCert = *flags.SubscriptionCert
	driver.PublishSettingsFilePath = *flags.PublishSettingsFilePath

	if (len(driver.SubscriptionID) == 0 || len(driver.SubscriptionCert) == 0) && len(driver.PublishSettingsFilePath) == 0 {
		return fmt.Errorf("Please specify azure subscription params using options: --azure-subscription-id and --azure-subscription-cert or --azure-publish-settings-file")
	}

	if len(*flags.Name) == 0 {
		driver.Name = generateVMName()
	} else {
		driver.Name = *flags.Name
	}

	driver.Location = *flags.Location

	if *flags.Size != "ExtraSmall" && *flags.Size != "Small" && *flags.Size != "Medium" &&
		*flags.Size != "Large" && *flags.Size != "ExtraLarge" &&
		*flags.Size != "A5" && *flags.Size != "A6" && *flags.Size != "A7" {
		return fmt.Errorf("Invalid VM size specified with --azure-size. Allowed values are 'ExtraSmall,Small,Medium,Large,ExtraLarge,A5,A6,A7.")
	}

	driver.Size = *flags.Size
	if strings.ToLower(*flags.UserName) == "docker" {
		return fmt.Errorf("'docker' is not valid user name for docker host. Please specify another user name.")
	} else {
		driver.UserName = *flags.UserName
	}
	driver.UserPassword = *flags.UserPassword
	driver.Image = *flags.Image
	driver.DockerCertDir = *flags.DockerCertDir

	dockerPort, err := strconv.Atoi(*flags.DockerPort)
	if err != nil {
		return err
	}
	driver.DockerPort = dockerPort

	sshPort, err := strconv.Atoi(*flags.SshPort)
	if err != nil {
		return err
	}
	driver.SshPort = sshPort

	return nil
}

func (driver *Driver) Create() error {
	err := createAzureVM(driver)
	if err != nil {
		return err
	}

	return nil
}

func (driver *Driver) GetURL() (string, error) {
	url := fmt.Sprintf("tcp://%s:%v", driver.Name+".cloudapp.net", driver.DockerPort)
	return url, nil
}

func (driver *Driver) GetIP() (string, error) {
	err := driver.setUserSubscription()
	if err != nil {
		return "", err
	}
	dockerVM, err := vmClient.GetVMDeployment(driver.Name, driver.Name)
	if err != nil {
		if strings.Contains(err.Error(), "Code: ResourceNotFound") {
			return "", fmt.Errorf("Azure host was not found. Please check your Azure subscription.")
		}
		return "", err
	}
	vip := dockerVM.RoleList.Role[0].ConfigurationSets.ConfigurationSet[0].InputEndpoints.InputEndpoint[0].Vip

	return vip, nil
}

func (driver *Driver) GetState() (state.State, error) {
	err := driver.setUserSubscription()
	if err != nil {
		return state.None, err
	}

	dockerVM, err := vmClient.GetVMDeployment(driver.Name, driver.Name)
	if err != nil {
		if strings.Contains(err.Error(), "Code: ResourceNotFound") {
			return state.None, fmt.Errorf("Azure host was not found. Please check your Azure subscription.")
		}

		return state.None, err
	}

	vmState := dockerVM.RoleInstanceList.RoleInstance[0].PowerState
	switch vmState {
	case "Started":
		return state.Running, nil
	case "Starting":
		return state.Starting, nil
	case "Stopped":
		return state.Stopped, nil
	}

	return state.None, nil
}

func (driver *Driver) Start() error {
	err := driver.setUserSubscription()
	if err != nil {
		return err
	}

	vmState, err := driver.GetState()
	if err != nil {
		return err
	}
	if vmState == state.Running || vmState == state.Starting {
		fmt.Println("Azure host is already running or starting.")
		return nil
	}

	err = vmClient.StartRole(driver.Name, driver.Name, driver.Name)
	if err != nil {
		return err
	}
	err = driver.waitForDocker()
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) Stop() error {
	err := driver.setUserSubscription()
	if err != nil {
		return err
	}
	vmState, err := driver.GetState()
	if err != nil {
		return err
	}
	if vmState == state.Stopped {
		fmt.Println("Azure host is already stopped.")
		return nil
	}
	err = vmClient.ShutdownRole(driver.Name, driver.Name, driver.Name)
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) Remove() error {
	err := driver.setUserSubscription()
	if err != nil {
		return err
	}

	_, err = vmClient.GetVMDeployment(driver.Name, driver.Name)
	if err != nil {
		if strings.Contains(err.Error(), "Code: ResourceNotFound") {
			return nil
		}

		return err
	}

	err = vmClient.DeleteVMDeployment(driver.Name, driver.Name)
	if err != nil {
		return err
	}
	err = vmClient.DeleteHostedService(driver.Name)
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) Restart() error {
	err := driver.setUserSubscription()
	if err != nil {
		return err
	}
	vmState, err := driver.GetState()
	if err != nil {
		return err
	}
	if vmState == state.Stopped {
		fmt.Println("Azure host is already stopped, use start command to run it.")
		return nil
	}
	err = vmClient.RestartRole(driver.Name, driver.Name, driver.Name)
	if err != nil {
		return err
	}
	err = driver.waitForDocker()
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) Kill() error {
	err := driver.setUserSubscription()
	if err != nil {
		return err
	}
	vmState, err := driver.GetState()
	if err != nil {
		return err
	}
	if vmState == state.Stopped {
		fmt.Println("Azure host is already stopped.")
		return nil
	}
	err = vmClient.ShutdownRole(driver.Name, driver.Name, driver.Name)
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	err := driver.setUserSubscription()
	if err != nil {
		fmt.Println(err)
	}

	vmState, err := driver.GetState()
	if err != nil {
		fmt.Println(err)
	}

	if vmState == state.Stopped {
		fmt.Println("Azure host is stopped. Please start it before using ssh command.")
	}

	return ssh.GetSSHCommand(driver.Name+".cloudapp.net", driver.SshPort, driver.UserName, driver.sshKeyPath(), args...)
}

//Region public methods ends

//Region private methods starts

func createAzureVM(driver *Driver) error {

	err := driver.setUserSubscription()
	if err != nil {
		return err
	}

	vmConfig, err := vmClient.CreateAzureVMConfiguration(driver.Name, driver.Size, driver.Image, driver.Location)
	if err != nil {
		return err
	}

	err = driver.generateCertForAzure()
	if err != nil {
		return err
	}

	vmConfig, err = vmClient.AddAzureLinuxProvisioningConfig(vmConfig, driver.UserName, driver.UserPassword, driver.azureCertPath(), driver.SshPort)
	if err != nil {
		return err
	}

	vmConfig, err = vmClient.SetAzureDockerVMExtension(vmConfig, driver.DockerCertDir, driver.DockerPort, "0.3")
	if err != nil {
		return err
	}

	err = vmClient.CreateAzureVM(vmConfig, driver.Name, driver.Location)
	if err != nil {
		return err
	}

	err = driver.waitForDocker()
	if err != nil {
		return err
	}

	return nil
}

func generateVMName() string {
	randomId := utils.TruncateID(utils.GenerateRandomID())
	return fmt.Sprintf("docker-host-%s", randomId)
}

func (driver *Driver) setUserSubscription() error {
	if len(driver.PublishSettingsFilePath) != 0 {
		err := azure.ImportPublishSettingsFile(driver.PublishSettingsFilePath)
		if err != nil {
			return err
		}
		return nil
	}
	err := azure.ImportPublishSettings(driver.SubscriptionID, driver.SubscriptionCert)
	if err != nil {
		return err
	}
	return nil
}

func (driver *Driver) waitForDocker() error {
	fmt.Println("Waiting for docker daemon on remote machine to be available.")
	maxRepeats := 24
	url := fmt.Sprintf("http://%s:%v", driver.Name+".cloudapp.net", driver.DockerPort)
	success := waitForDockerEndpoint(url, maxRepeats)
	if !success {
		fmt.Print("\n")
		fmt.Println("Restarting docker daemon on remote machine.")
		err := vmClient.RestartRole(driver.Name, driver.Name, driver.Name)
		if err != nil {
			return err
		}
		success = waitForDockerEndpoint(url, maxRepeats)
		if !success {
			fmt.Print("\n")
			fmt.Println("Error: Can not run docker daemon on remote machine. Please check docker daemon at " + url)
		}
	}
	fmt.Println()
	fmt.Println("Docker daemon is ready.")
	return nil
}

func waitForDockerEndpoint(url string, maxRepeats int) bool {
	counter := 0
	for {
		resp, err := http.Get(url)
		error := err.Error()
		if strings.Contains(error, "malformed HTTP response") || len(error) == 0 {
			break
		}
		fmt.Print(".")
		if resp != nil {
			fmt.Println(resp)
		}
		time.Sleep(10 * time.Second)
		counter++
		if counter == maxRepeats {
			return false
		}
	}
	return true
}

func (driver *Driver) generateCertForAzure() error {
	if err := ssh.GenerateSSHKey(driver.sshKeyPath()); err != nil {
		return err
	}

	cmd := exec.Command("openssl", "req", "-x509", "-key", driver.sshKeyPath(), "-nodes", "-days", "365", "-newkey", "rsa:2048", "-out", driver.azureCertPath(), "-subj", "/C=AU/ST=Some-State/O=InternetWidgitsPtyLtd/CN=\\*")
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (driver *Driver) sshKeyPath() string {
	return path.Join(driver.storePath, "id_rsa")
}

func (driver *Driver) publicSSHKeyPath() string {
	return driver.sshKeyPath() + ".pub"
}

func (driver *Driver) azureCertPath() string {
	return path.Join(driver.storePath, "azure_cert.pem")
}
