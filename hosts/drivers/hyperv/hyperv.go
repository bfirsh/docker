package hyperv

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/docker/docker/hosts/drivers"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/pkg/log"
	"github.com/docker/docker/utils"
	flag "github.com/docker/docker/pkg/mflag"
)

type Driver struct {
	storePath      string
	Boot2DockerURL string
	Boot2DockerLoc string
	VSwitch        string
	MachineName    string
}

type CreateFlags struct {
	Boot2DockerURL *string
	Boot2DockerLoc *string
	VSwitch        *string
}

func init() {
	drivers.Register("hyperv", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.Boot2DockerURL = cmd.String([]string{"-hyperv-boot2docker-url"}, "", "The URL of the boot2docker image. Defaults to the latest available version")
	createFlags.Boot2DockerLoc = cmd.String([]string{"-hyperv-boot2docker-location"}, "", "Local boot2docker iso.")
	createFlags.VSwitch = cmd.String([]string{"-hyperv-virtual-switch"}, "", "Name of virtual switch. Defaults to first found.")
	return createFlags
}

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func (d *Driver) DriverName() string {
	return "hyperv"
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	if ip == "" {
		return "", nil
	}
	return fmt.Sprintf("tcp://%s:2376", ip), nil
}

func (d *Driver) GetState() (state.State, error) {

	command := []string{
		"(",
		"Get-VM",
		"-Name", "'" + d.MachineName + "'",
		").state"}
	stdout, err := execute(command)
	if err != nil {
		return state.None, fmt.Errorf("Failed to find the VM status")
	}
	resp := parseStdout(stdout)

	if len(resp) < 1 {
		return state.None, nil
	}
	switch resp[0] {
	case "Running":
		return state.Running, nil
	case "Off":
		return state.Stopped, nil
	}
	return state.None, nil
}

func copyFile(inFile, outFile string) error {
	in, err := os.Open(inFile)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	err = out.Sync()
	return err
}

func (d *Driver) Create() error {
	err := hypervAvailable()
	if err != nil {
		return err
	}

	d.setMachineNameIfNotSet()

	var isoURL string

	if d.Boot2DockerLoc == "" {
		if d.Boot2DockerURL != "" {
			isoURL = d.Boot2DockerURL
		} else {
			// HACK: Docker 1.3 boot2docker image
			isoURL = "http://cl.ly/1c1c0O3N193A/download/boot2docker-1.2.0-dev.iso"
			// isoURL, err = getLatestReleaseURL()
			// if err != nil {
			// 	return err
			// }
		}
		log.Infof("Downloading boot2docker...")

		if err := downloadISO(d.storePath, "boot2docker.iso", isoURL); err != nil {
			return err
		}
	} else {
		copyFile(d.Boot2DockerLoc, path.Join(d.storePath, "boot2docker.iso"))
	}

	log.Infof("Creating SSH key...")

	if err := ssh.GenerateSSHKey(d.sshKeyPath()); err != nil {
		return err
	}

	log.Infof("Creating  VM...")

	//Get which virtual switch to use from the user
	virtualSwitch, err := d.chooseVirtualSwitch()
	if err != nil {
		return err
	}

	//hardcoded to 1G at the moment
	memorySizeInBytes := fmt.Sprintf("%d", (1024 * 1024 * 1024))

	log.Infof("Creating a new Virtual Machine")
	command := []string{
		"New-VM",
		"-Name", "'" + d.MachineName + "'",
		"-Path", "'" + d.storePath + "'",
		"-MemoryStartupBytes", memorySizeInBytes}
	_, err = execute(command)
	if err != nil {
		return err
	}

	command = []string{
		"Set-VMDvdDrive",
		"-VMName", "'" + d.MachineName + "'",
		"-Path", "'" + path.Join(d.storePath, "boot2docker.iso") + "'"}
	_, err = execute(command)
	if err != nil {
		return err
	}

	command = []string{
		"Connect-VMNetworkAdapter",
		"-VMName", "'" + d.MachineName + "'",
		"-SwitchName", "'" + virtualSwitch + "'"}
	_, err = execute(command)
	if err != nil {
		return err
	}

	log.Infof("Starting  VM...")
	return d.Start()
}

func (d *Driver) chooseVirtualSwitch() (string, error) {
	if d.VSwitch != "" {
		return d.VSwitch, nil
	}
	command := []string{
		"@(Get-VMSwitch).Name"}
	stdout, err := execute(command)
	if err != nil {
		return "", err
	}
	switches := parseStdout(stdout)
	if len(switches) > 0 {
		log.Infof("Using switch %s", switches[0])
		return switches[0], nil
	}
	return "", fmt.Errorf("no vswitch found")
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	d.Boot2DockerURL = *flags.Boot2DockerURL
	d.Boot2DockerLoc = *flags.Boot2DockerLoc
	d.VSwitch = *flags.VSwitch
	return nil
}

func (d *Driver) wait() error {
	log.Infof("Waiting for host to start...")
	for {
		ip, _ := d.GetIP()
		if ip != "" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	log.Infof("Got IP, waiting for SSH")
	ip, _ := d.GetIP()
	return ssh.WaitForTCP(fmt.Sprintf("%s:22", ip))
}

func (d *Driver) Start() error {
	command := []string{
		"Start-VM",
		"-Name", "'" + d.MachineName + "'"}
	_, err := execute(command)
	if err != nil {
		return err
	}
	return d.wait()
}

func (d *Driver) Stop() error {
	command := []string{
		"Stop-VM",
		"-Name", "'" + d.MachineName + "'"}
	_, err := execute(command)
	if err != nil {
		return err
	}
	for {
		s, err := d.GetState()
		if err != nil {
			return err
		}
		if s == state.Running {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	return nil
}

func (d *Driver) Remove() error {
	s, err := d.GetState()
	if err != nil {
		return err
	}
	if s == state.Running {
		if err := d.Kill(); err != nil {
			return err
		}
	}
	command := []string{
		"Remove-VM",
		"-Name", "'" + d.MachineName + "'",
		"-Force"}
	_, err = execute(command)
	return err
}

func (d *Driver) Restart() error {
	command := []string{
		"Restart-VM",
		"-Name", "'" + d.MachineName + "'",
		"-Force"}
	_, err := execute(command)
	if err != nil {
		return err
	}
	return d.wait()
}

func (d *Driver) Kill() error {
	return d.Stop()
}

func (d *Driver) setMachineNameIfNotSet() {
	if d.MachineName == "" {
		d.MachineName = fmt.Sprintf("docker-host-%s", utils.TruncateID(utils.GenerateRandomID()))
	}
}

func (d *Driver) GetIP() (string, error) {
	command := []string{
		"((",
		"Get-VM",
		"-Name", "'" + d.MachineName + "'",
		").networkadapters[0]).ipaddresses[0]"}
	stdout, err := execute(command)
	if err != nil {
		return "", err
	}
	resp := parseStdout(stdout)
	if len(resp) < 1 {
		return "", fmt.Errorf("IP not found")
	}
	return resp[0], nil
}

func (d *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	ip, _ := d.GetIP()
	return ssh.GetSSHCommand(ip, 22, "docker", d.sshKeyPath(), args...)
}

func (d *Driver) sshKeyPath() string {
	return path.Join(d.storePath, "id_rsa")
}

func (d *Driver) publicSSHKeyPath() string {
	return d.sshKeyPath() + ".pub"
}

// Get the latest boot2docker release tag name (e.g. "v0.6.0").
// FIXME: find or create some other way to get the "latest release" of boot2docker since the GitHub API has a pretty low rate limit on API requests
// func getLatestReleaseURL() (string, error) {
// 	rsp, err := http.Get("https://api.github.com/repos/boot2docker/boot2docker/releases")
// 	if err != nil {
// 		return "", err
// 	}
// 	defer rsp.Body.Close()

// 	var t []struct {
// 		TagName string `json:"tag_name"`
// 	}
// 	if err := json.NewDecoder(rsp.Body).Decode(&t); err != nil {
// 		return "", err
// 	}
// 	if len(t) == 0 {
// 		return "", fmt.Errorf("no releases found")
// 	}

// 	tag := t[0].TagName
// 	url := fmt.Sprintf("https://github.com/boot2docker/boot2docker/releases/download/%s/boot2docker.iso", tag)
// 	return url, nil
// }

// Download boot2docker ISO image for the given tag and save it at dest.
func downloadISO(dir, file, url string) error {
	rsp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	// Download to a temp file first then rename it to avoid partial download.
	f, err := ioutil.TempFile(dir, file+".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, rsp.Body); err != nil {
		// TODO: display download progress?
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path.Join(dir, file)); err != nil {
		return err
	}
	return nil
}

// // Make a boot2docker VM disk image.
// func (d *Driver) generateDiskImage(size int) error {
// 	log.Debugf("Creating %d MB hard disk image...", size)

// 	magicString := "boot2docker, please format-me"

// 	buf := new(bytes.Buffer)
// 	tw := tar.NewWriter(buf)

// 	// magicString first so the automount script knows to format the disk
// 	file := &tar.Header{Name: magicString, Size: int64(len(magicString))}
// 	if err := tw.WriteHeader(file); err != nil {
// 		return err
// 	}
// 	if _, err := tw.Write([]byte(magicString)); err != nil {
// 		return err
// 	}
// 	// .ssh/key.pub => authorized_keys
// 	file = &tar.Header{Name: ".ssh", Typeflag: tar.TypeDir, Mode: 0700}
// 	if err := tw.WriteHeader(file); err != nil {
// 		return err
// 	}
// 	pubKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
// 	if err != nil {
// 		return err
// 	}
// 	file = &tar.Header{Name: ".ssh/authorized_keys", Size: int64(len(pubKey)), Mode: 0644}
// 	if err := tw.WriteHeader(file); err != nil {
// 		return err
// 	}
// 	if _, err := tw.Write([]byte(pubKey)); err != nil {
// 		return err
// 	}
// 	file = &tar.Header{Name: ".ssh/authorized_keys2", Size: int64(len(pubKey)), Mode: 0644}
// 	if err := tw.WriteHeader(file); err != nil {
// 		return err
// 	}
// 	if _, err := tw.Write([]byte(pubKey)); err != nil {
// 		return err
// 	}
// 	if err := tw.Close(); err != nil {
// 		return err
// 	}
// 	raw := bytes.NewReader(buf.Bytes())
// 	return createDiskImage(d.diskPath(), size, raw)
// }

// // createDiskImage makes a disk image at dest with the given size in MB. If r is
// // not nil, it will be read as a raw disk image to convert from.
// func createDiskImage(dest string, size int, r io.Reader) error {
// 	// Convert a raw image from stdin to the dest VMDK image.
// 	sizeBytes := int64(size) << 20 // usually won't fit in 32-bit int (max 2GB)
// 	// FIXME: why isn't this just using the vbm*() functions?
// 	cmd := exec.Command(vboxManageCmd, "convertfromraw", "stdin", dest,
// 		fmt.Sprintf("%d", sizeBytes), "--format", "VMDK")

// 	if os.Getenv("DEBUG") != "" {
// 		cmd.Stdout = os.Stdout
// 		cmd.Stderr = os.Stderr
// 	}

// 	stdin, err := cmd.StdinPipe()
// 	if err != nil {
// 		return err
// 	}
// 	if err := cmd.Start(); err != nil {
// 		return err
// 	}

// 	n, err := io.Copy(stdin, r)
// 	if err != nil {
// 		return err
// 	}

// 	// The total number of bytes written to stdin must match sizeBytes, or
// 	// VBoxManage.exe on Windows will fail. Fill remaining with zeros.
// 	if left := sizeBytes - n; left > 0 {
// 		if err := zeroFill(stdin, left); err != nil {
// 			return err
// 		}
// 	}

// 	// cmd won't exit until the stdin is closed.
// 	if err := stdin.Close(); err != nil {
// 		return err
// 	}

// 	return cmd.Wait()
// }

// // zeroFill writes n zero bytes into w.
// func zeroFill(w io.Writer, n int64) error {
// 	const blocksize = 32 << 10
// 	zeros := make([]byte, blocksize)
// 	var k int
// 	var err error
// 	for n > 0 {
// 		if n > blocksize {
// 			k, err = w.Write(zeros)
// 		} else {
// 			k, err = w.Write(zeros[:n])
// 		}
// 		if err != nil {
// 			return err
// 		}
// 		n -= int64(k)
// 	}
// 	return nil
// }
