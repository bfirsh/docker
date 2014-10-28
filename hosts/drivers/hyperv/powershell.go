package hyperv

import (
	"bufio"
	"bytes"
	"errors"
	util "github.com/boot2docker/boot2docker-cli/util"
	"os/exec"
	"regexp"
	"strings"
	"os"
	"path/filepath"
	"github.com/docker/docker/pkg/log"
)

var (
	reMachineNotFound = regexp.MustCompile(`Hyper-V was unable to find a virtual machine with name (.+)`)
)

var (
	ErrMachineExist    = errors.New("machine already exists")
	ErrMachineNotExist = errors.New("machine does not exist")
	ErrVBMNotFound     = errors.New("Powershell not found")
)

var vbm string

func init() {
	systemPath := strings.Split(os.Getenv("PATH"), ";")
	for _, path := range systemPath {
		if strings.Index(path, "WindowsPowerShell") != -1 {
			vbm = filepath.Join(path, "powershell.exe")
		}
	}
}

func execute(args []string) (string, error) {
	cmd := exec.Command(vbm, args...)
 	log.Debugf("[executing ==>] : %v %v", vbm, strings.Join(args, " "))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
 	log.Debugf("[stdout =====>] : %s", stdout.String())
 	log.Debugf("[stderr =====>] : %s", stderr.String())
	return stdout.String(), err
}

func checkForAdminAccess() bool {
	command := []string{
		"(",
		"New-Object",
		"Security.Principal.WindowsPrincipal",
		"$([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator",
		")"}
	stdout, err := execute(command)

	if err != nil {
		util.Logf("The following error occurred %s", err)
		return false
	}

	resp := parseStdout(stdout)
	if resp[0] == "False" {
		util.Logf("You need to have Administrator privilege to access this command")
		return false
	}
	return true
}

func parseStdout(stdout string) []string {
	s := bufio.NewScanner(strings.NewReader(stdout))
	resp := []string{}
	for s.Scan() {
		resp = append(resp, s.Text())
	}
	return resp
}

func hypervAvailable() bool {
	command := []string{
		"@(Get-Command Get-VM).ModuleName"}
	stdout, err := execute(command)
	if err != nil {
		util.Logf("Hyper-V is not available in this machine. Please enable it from Windows Features.")
		return false
	}
	resp := parseStdout(stdout)

	if resp[0] == "Hyper-V" {
		return true
	} else {
		util.Logf("Hyper-V PowerShel Module is not available")
		return false
	}
}
