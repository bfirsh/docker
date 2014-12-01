/*
 * Copyright 2014 VMware, Inc.  All rights reserved.  Licensed under the Apache v2 License.
 */

package vsphere

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/docker/docker/pkg/log"
)

var (
	GovcCmd = "govc"
)

func govc(args ...string) error {
	cmd := exec.Command(GovcCmd, args...)

	log.Debugf("executing: %v %v", GovcCmd, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func govcOutErr(args ...string) (string, string, error) {
	cmd := exec.Command(GovcCmd, args...)

	log.Debugf("executing: %v %v", GovcCmd, strings.Join(args, " "))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
