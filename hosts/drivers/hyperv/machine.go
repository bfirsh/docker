package hyperv

func getMachine(name string) error {
	command := []string{"Get-VM", "-Name", name}
	_, err := execute(command)
	return err
}

func machineExists(name string) bool {
	err := getMachine(name)
	if err != nil {
		return false
	}
	return true
}
