package gce

import (
	"flag"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/docker/docker/hosts/state"
)

var (
	project = flag.String("project", "", "Project")
)

var (
	tmpDir string
	driver *Driver
	c      *ComputeUtil
	zone   = "us-central1-a"
)

func init() {
	flag.Parse()

	if *project == "" {
		log.Fatal("You must specify a GCE project using the --project flag.")
	}

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}

	driver = &Driver{
		storePath:        tmpDir,
		InstanceName:     "test-instance",
		Zone:             "us-central1-a",
		MachineType:      "n1-standard-1",
		UserName:         os.Getenv("USER"),
		Project:          *project,
		sshKeyPath:       path.Join(tmpDir, "id_rsa"),
		publicSSHKeyPath: path.Join(tmpDir, "id_rsa.pub"),
	}
	c, err = newComputeUtil(driver)
	if err != nil {
		log.Fatal(err)
	}
}

func cleanupDisk() {
	log.Println("Cleaning up disk.")
	d, err := c.service.Disks.Get(*project, zone, "test-instance-disk").Do()
	if d == nil {
		return
	}
	op, err := c.service.Disks.Delete(*project, zone, "test-instance-disk").Do()
	if err != nil {
		log.Printf("Error cleaning up disk: %v", err)
		return
	}
	err = c.waitForOp(op.Name)
	if err != nil {
		log.Printf("Error cleaning up disk: %v", err)
	}
}

func cleanupInstance() {
	log.Println("Cleaning up instance.")
	d, err := c.service.Instances.Get(*project, zone, "test-instance").Do()
	if d == nil {
		return
	}
	op, err := c.service.Instances.Delete(*project, zone, "test-instance").Do()
	if err != nil {
		log.Printf("Error cleaning up instance: %v", err)
		return
	}
	err = c.waitForOp(op.Name)
	if err != nil {
		log.Printf("Error cleaning up instance: %v", err)
	}
}

func cleanup() {
	cleanupInstance()
	cleanupDisk()
}

func TestBasicOperations(t *testing.T) {
	log.Info("Create")
	err := driver.Create()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	AssertDiskAndInstance(true, true)
	s, _ := driver.GetState()
	if s != state.Running {
		t.Fatalf("State should be Running, but is: %v", s)
	}

	log.Info("Stop")
	err = driver.Stop()
	if err != nil {
		t.Fatal(err)
	}
	AssertDiskAndInstance(true, false)
	s, _ = driver.GetState()
	if s != state.Stopped {
		t.Fatalf("State should be Stopped, but is: %v", s)
	}

	log.Info("Start")
	err = driver.Start()
	if err != nil {
		t.Fatal(err)
	}
	AssertDiskAndInstance(true, true)
	s, _ = driver.GetState()
	if s != state.Running {
		t.Fatalf("State should be Running, but is: %v", s)
	}

	log.Info("Restart")
	err = driver.Restart()
	if err != nil {
		t.Fatal(err)
	}
	AssertDiskAndInstance(true, true)
	s, _ = driver.GetState()
	if s != state.Running {
		t.Fatalf("State should be Running, but is: %v", s)
	}

	log.Info("Remove")
	err = driver.Remove()
	if err != nil {
		t.Fatal(err)
	}
	AssertDiskAndInstance(false, false)
	s, _ = driver.GetState()
	if s != state.None {
		t.Fatalf("State should be None, but is: %v", s)
	}
}

func AssertDiskAndInstance(diskShouldExist, instShouldExist bool) {
	d, err := c.service.Disks.Get(*project, zone, "test-instance-disk").Do()
	if diskShouldExist {
		if d == nil || err != nil {
			log.Fatal("Error retrieving disk that should exist.")
		}
	} else if d != nil {
		log.Fatal("Disk shouldn't exist but does.")
	}
	i, err := c.service.Instances.Get(*project, zone, "test-instance").Do()
	if instShouldExist {
		if i == nil || err != nil {
			log.Fatal("error retrieving instance that should exist.")
		}
	} else if i != nil {
		log.Fatal("Instance shouldnt exist but does.")
	}
}
