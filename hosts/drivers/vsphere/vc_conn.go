/*
 * Copyright 2014 VMware, Inc.  All rights reserved.  Licensed under the Apache v2 License.
 */

package vsphere

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/hosts/drivers/vsphere/errors"
	"github.com/docker/docker/pkg/log"
)

type VcConn struct {
	driver *Driver
}

func NewVcConn(driver *Driver) VcConn {
	return VcConn{driver: driver}
}

func (conn VcConn) DatastoreLs(path string) (string, error) {
	args := []string{"datastore.ls"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--ds=%s", conn.driver.Datastore))
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, path)
	stdout, stderr, err := govcOutErr(args...)
	if stderr == "" && err == nil {
		return stdout, nil
	}
	return "", errors.NewDatastoreError(conn.driver.Datastore, "ls", stderr)
}

func (conn VcConn) DatastoreMkdir(dirName string) error {
	_, err := conn.DatastoreLs(dirName)
	if err == nil {
		return nil
	}

	log.Infof("Creating directory %s on datastore %s of vCenter %s... ",
		dirName, conn.driver.Datastore, conn.driver.IP)

	args := []string{"datastore.mkdir"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--ds=%s", conn.driver.Datastore))
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, dirName)
	_, stderr, err := govcOutErr(args...)
	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewDatastoreError(conn.driver.Datastore, "mkdir", stderr)
	}
}

func (conn VcConn) DatastoreUpload(localPath string) error {
	stdout, err := conn.DatastoreLs(DATASTORE_DIR)
	if err == nil && strings.Contains(stdout, B2D_ISO_NAME) {
		log.Infof("boot2docker ISO already uploaded, skipping upload... ")
		return nil
	}

	log.Infof("Uploading %s to %s on datastore %s of vCenter %s... ",
		localPath, DATASTORE_DIR, conn.driver.Datastore, conn.driver.IP)

	dsPath := fmt.Sprintf("%s/%s", DATASTORE_DIR, B2D_ISO_NAME)
	args := []string{"datastore.upload"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--ds=%s", conn.driver.Datastore))
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, localPath)
	args = append(args, dsPath)
	_, stderr, err := govcOutErr(args...)
	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewDatastoreError(conn.driver.Datacenter, "upload", stderr)
	}
}

func (conn VcConn) VmInfo() (string, error) {
	args := []string{"vm.info"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, conn.driver.MachineName)

	stdout, stderr, err := govcOutErr(args...)
	if strings.Contains(stdout, "Name") && stderr == "" && err == nil {
		return stdout, nil
	} else {
		return "", errors.NewVmError("find", conn.driver.MachineName, "VM not found")
	}
}

func (conn VcConn) VmCreate(isoPath string) error {
	log.Infof("Creating virtual machine %s of vCenter %s... ",
		conn.driver.MachineName, conn.driver.IP)

	args := []string{"vm.create"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--net=%s", conn.driver.Network))
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--ds=%s", conn.driver.Datastore))
	args = append(args, fmt.Sprintf("--iso=%s", isoPath))
	memory := strconv.Itoa(conn.driver.Memory)
	args = append(args, fmt.Sprintf("--m=%s", memory))
	cpu := strconv.Itoa(conn.driver.CPU)
	args = append(args, fmt.Sprintf("--c=%s", cpu))
	args = append(args, "--disk.controller=scsi")
	args = append(args, "--on=false")
	if conn.driver.Pool != "" {
		args = append(args, fmt.Sprintf("--pool=%s", conn.driver.Pool))
	}
	if conn.driver.HostIP != "" {
		args = append(args, fmt.Sprintf("--host.ip=%s", conn.driver.HostIP))
	}
	args = append(args, conn.driver.MachineName)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("create", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) VmPowerOn() error {
	log.Infof("Powering on virtual machine %s of vCenter %s... ",
		conn.driver.MachineName, conn.driver.IP)

	args := []string{"vm.power"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, "-on")
	args = append(args, conn.driver.MachineName)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("power on", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) VmPowerOff() error {
	log.Infof("Powering off virtual machine %s of vCenter %s... ",
		conn.driver.MachineName, conn.driver.IP)

	args := []string{"vm.power"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, "-off")
	args = append(args, conn.driver.MachineName)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("power on", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) VmDestroy() error {
	log.Infof("Deleting virtual machine %s of vCenter %s... ",
		conn.driver.MachineName, conn.driver.IP)

	args := []string{"vm.destroy"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, conn.driver.MachineName)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("delete", conn.driver.MachineName, stderr)
	}

}

func (conn VcConn) VmDiskCreate() error {
	args := []string{"vm.disk.create"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--vm=%s", conn.driver.MachineName))
	args = append(args, fmt.Sprintf("--ds=%s", conn.driver.Datastore))
	args = append(args, fmt.Sprintf("--name=%s", conn.driver.MachineName))
	diskSize := strconv.Itoa(conn.driver.DiskSize)
	args = append(args, fmt.Sprintf("--size=%sMiB", diskSize))

	_, stderr, err := govcOutErr(args...)
	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("add network", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) VmAttachNetwork() error {
	args := []string{"vm.network.add"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--vm=%s", conn.driver.MachineName))
	args = append(args, fmt.Sprintf("--net=%s", conn.driver.Network))

	_, stderr, err := govcOutErr(args...)
	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewVmError("add network", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) VmFetchIp() (string, error) {
	args := []string{"vm.ip"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, conn.driver.MachineName)
	stdout, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return stdout, nil
	} else {
		return "", errors.NewVmError("fetching IP", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) GuestMkdir(guestUser, guestPass, dirName string) error {
	args := []string{"guest.mkdir"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--l=%s:%s", guestUser, guestPass))
	args = append(args, fmt.Sprintf("--vm=%s", conn.driver.MachineName))
	args = append(args, "-p")
	args = append(args, dirName)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewGuestError("mkdir", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) GuestUpload(guestUser, guestPass, localPath, remotePath string) error {
	args := []string{"guest.upload"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--l=%s:%s", guestUser, guestPass))
	args = append(args, fmt.Sprintf("--vm=%s", conn.driver.MachineName))
	args = append(args, "-f")
	args = append(args, localPath)
	args = append(args, remotePath)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewGuestError("upload", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) GuestDownload(guestUser, guestPass, remotePath, localPath string) error {
	args := []string{"guest.download"}
	args = conn.AppendConnectionString(args)
	args = append(args, fmt.Sprintf("--dc=%s", conn.driver.Datacenter))
	args = append(args, fmt.Sprintf("--l=%s:%s", guestUser, guestPass))
	args = append(args, fmt.Sprintf("--vm=%s", conn.driver.MachineName))
	args = append(args, remotePath)
	args = append(args, localPath)
	_, stderr, err := govcOutErr(args...)

	if stderr == "" && err == nil {
		return nil
	} else {
		return errors.NewGuestError("download", conn.driver.MachineName, stderr)
	}
}

func (conn VcConn) AppendConnectionString(args []string) []string {
	args = append(args, fmt.Sprintf("--u=%s:%s@%s", conn.driver.Username, conn.driver.Password, conn.driver.IP))
	args = append(args, "--k=true")
	return args
}
