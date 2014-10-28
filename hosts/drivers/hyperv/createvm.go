package hyperv

import (
	"fmt"

	"github.com/docker/docker/pkg/log"
)

func createVM(iso, name, dir string) error {

	//Get which virtual switch to use from the user
	virtualSwitch, err := chooseVirtualSwitch()
	if err != nil {
		return err
	}

	// =============================
	// Create a new Virtual Machine
	// =============================
	// Convert Disk size from MB to Bytes and to a String
	memorySizeInBytes := fmt.Sprintf("%d", (1024 * 1024 * 1024))

	log.Infof("Creating a new Virtual Machine")
	command := []string{
		"New-VM",
		"-Name", "'" + name + "'",
		"-Path", "'" + dir + "'",
		"-MemoryStartupBytes", memorySizeInBytes}

	_, err = execute(command)
	if err != nil {
		return err
	}

	// Add a Hard Disk and ISO file to this VM
	// Add ISO
	log.Infof("Adding ISO file and VHDX.")
	command = []string{
		"Set-VMDvdDrive",
		"-VMName", "'" + name + "'",
		"-Path", "'" + iso + "'"}
	_, err = execute(command)
	if err != nil {
		return err
	}

	log.Infof("Connecting network adapter to virtual switch")
	command = []string{
		"Connect-VMNetworkAdapter",
		"-VMName", "'" + name + "'",
		"-SwitchName", "'" + virtualSwitch + "'"}
	_, err = execute(command)
	if err != nil {
		return err
	}

	log.Infof("All Done!! Successfully created a Virtual Machine")

	return nil
}

func chooseVirtualSwitch() (string, error) {
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

	} else {
		return "", fmt.Errorf("no vswitch found")
	}
}
