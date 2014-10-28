package hyperv

func startInstance(name string) error {
	command := []string{
		"Start-VM",
		"-Name", "'" + name + "'"}
	_, err := execute(command)
	return err
}

func resumeMachine(name string) error {
	command := []string{
		"Resume-VM",
		"-Name", "'" + name + "'"}
	_, err := execute(command)
	return err
}

func stopMachine(name string) error {
	command := []string{
		"Stop-VM",
		"-Name", "'" + name + "'"}
	_, err := execute(command)
	return err
}

func saveMachine(name string) error {
	command := []string{
		"Save-VM",
		"-Name", "'" + name + "'"}
	_, err := execute(command)
	return err
}

func turnOffMachine(name string) error {
	command := []string{
		"Stop-VM",
		"-Name", "'" + name + "'",
		"-Force"}
	_, err := execute(command)
	return err
}

func restartMachine(name string) error {
	command := []string{
		"Restart-VM",
		"-Name", "'" + name + "'",
		"-Force"}
	_, err := execute(command)
	return err
}
