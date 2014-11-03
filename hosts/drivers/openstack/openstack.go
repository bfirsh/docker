package openstack

// *Fixme
// Temporarily using gophercloud from rackspace.
// this is just code bloat right now, will get functionality
// of openstack needed into a seperate smaller library
import (
	"fmt"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
	"io/ioutil"
	"os/exec"
	
	"github.com/docker/docker/pkg/log"
	"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"
	gophercloud "github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/compute/v2/flavors"
    "github.com/rackspace/gophercloud/openstack/compute/v2/images"
	"github.com/rackspace/gophercloud/openstack/compute/v2/servers"
	"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/rules"
)

type Driver struct {
	IdentityEndpoint  string
	Keypair			  string
	AvailabilityZone  string
	UserUUID		  int
	Username		  string
	Password      	  string
	TenantID		  string
	TenantName 		  string
	RegionID     	  string 
	RegionName        string	
	OpenstackVMID     int
	OpenstackVMName   string
	ImageID       	  string
	IPAddress   	  string
	Flavor        	  string
	FloatingIpNetwork string
	FloatingIpPort	  string
	SecurityGroup     string
	NovaNetwork		  bool
	storePath   	  string
}

type CreateFlags struct {
	IdentityEndpoint  *string
	Keypair			  *string
	Username		  *string
	Password      	  *string
	ImageID			  *string
	TenantID		  *string
	Flavor       	  *string
	FloatingIpNetwork *string
	FloatingIpPort	  *string
	SecurityGroup	  *string
	NovaNetwork		  *bool
}

func init() {
	drivers.Register("openstack", &drivers.RegisteredDriver{
		New:                 NewDriver,
		RegisterCreateFlags: RegisterCreateFlags,
	})
}

// RegisterCreateFlags registers the flags this driver adds to
// "docker hosts create"
func RegisterCreateFlags(cmd *flag.FlagSet) interface{} {
	createFlags := new(CreateFlags)
	createFlags.IdentityEndpoint = cmd.String(
		[]string{"-openstack-auth-endpoint"},
		"",
		"Openstack Authentication Endpoint",
	)
	createFlags.Keypair = cmd.String(
		[]string{"-openstack-keypair"},
		"",
		"Openstack Authentication Endpoint",
	)
	createFlags.Username = cmd.String(
		[]string{"-openstack-username"},
		"",
		"Openstack Username",
	)
	createFlags.Password = cmd.String(
		[]string{"-openstack-password"},
		"",
		"Openstack Password",
	)
	createFlags.TenantID = cmd.String(
		[]string{"-openstack-tenant-id"},
		"",
		"Openstack Tenant UUID",
	)
	createFlags.ImageID = cmd.String(
		[]string{"-openstack-image-id"},
		"",
		"Openstack Image UUID",
	)
	createFlags.Flavor = cmd.String(
		[]string{"-openstack-flavor"},
		"m1.small",
		"Openstack Flavor Setting",
	)
	createFlags.FloatingIpNetwork = cmd.String(
		[]string{"-openstack-floating-net"},
		"public",
		"Openstack Floating IP Network UUID",
	)
	createFlags.FloatingIpPort = cmd.String(
		[]string{"-openstack-floating-port"},
		"",
		"Openstack Floating IP Network UUID",
	)
	createFlags.SecurityGroup = cmd.String(
		[]string{"-openstack-security-group"},
		"default",
		"Openstack Flavor Setting",
	)
	createFlags.NovaNetwork = cmd.Bool(
		[]string{"-openstack-nova-net"},
		"false",
		"Using Openstack Nova Network?",
	)
	return createFlags
}	

func NewDriver(storePath string) (drivers.Driver, error) {
	return &Driver{storePath: storePath}, nil
}

func (d *Driver) DriverName() string {
	return "openstack"
}

func (d *Driver) SetConfigFromFlags(flagsInterface interface{}) error {
	flags := flagsInterface.(*CreateFlags)
	d.IdentityEndpoint = *flags.IdentityEndpoint
	d.Keypair = *flags.Keypair
	d.Username = *flags.Username
	d.Password = *flags.Password
	d.ImageID =  *flags.ImageID
	d.TenantID = *flags.TenantID
	d.Flavor = *flags.Flavor
	d.FloatingIpNetwork = *flags.FloatingIpNetwork
	d.FloatingIpPort = *flags.FloatingIpPort
	d.SecurityGroup = *flags.SecurityGroup
	d.NovaNetwork = *flags.NovaNetwork
	
	// *Fixme, think about adding the function
	// pts, err := openstack.AuthOptionsFromEnv()
	// from gophercloud that check for auth in the
	// environment.
	
	if d.IdentiryEndpoint == "" {
		return fmt.Errorf("openstack driver requires the --openstack-auth-endpoint option")
	} else {
		//TODO Check for correct URL format, think about 35357 or 5000 or other
		//endpoints that may be auth and could work.
	}
	if d.Keypair == "" {
		return fmt.Errorf("openstack driver requires the --openstack-keypair option")
	}
	
	if d.ImageID == "" {
		return fmt.Errorf("openstack driver requires the --openstack-image-id option")
	}
	
	if d.Username == "" {
		return fmt.Errorf("openstack driver requires the --openstack-username option")
	}
	
	if d.Password == "" {
		return fmt.Errorf("openstack driver requires the --openstack-password option")
	}
	
	if d.TenantID == "" {
		return fmt.Errorf("openstack driver requires the --openstack-tenant-id option")
	}
	// Flavor is defaulted to m1.small
	// FloatingIpNetwork defaulted to public
	// SecurityGroup defaulted to defualt
    // NovaNetwork defaulted to false
    if d.NovaNetwork {
    	log.Infof("Using Nova Network Config")
    } else {
    	if d.FloatingIpNetwork == "" {
			return fmt.Errorf("openstack driver requires the --openstack-floating-net option")
		}
    	if d.FloatingIpPort == "" {
			return fmt.Errorf("openstack driver requires the --openstack-floating-port option")
		}
    }
    
	return nil
}

func (d *Driver) Create() error {
	d.setOpenstackVMName()
	
	//Get SSH key from flags, or create one.
	//FixMe, OpenstackAPIs v2 don't let you specify the
	//ssh key!!? running cloud-init scripts instead
	//Load User Data for docker installation OR wait for SSH, 
	//run commands through SSH (digitalocean #156)
	udata := []byte(""+
	"#!/bin/bash\n"+
	"sudo echo -e 'docker\ndocker' | passwd root\n" +
	"sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9\n"+
	"sudo sh -c 'echo deb https://get.docker.com/ubuntu docker main > /etc/apt/sources.list.d/docker.list'\n" +
	"sudo apt-get update\n" +
	"sudo apt-get install lxc-docker\n" +
	"sudo service lxc-docker stop\n" +
	"sudo service ufw stop\n" +
	"sudo docker -d -H tcp://0.0.0.0:2375 &\n")
	
	/* Connect to Endpoint
	   Authenticate
	   Get compute client */
	client := d.getClient()
	
	// TODO *FixMe Verify image, flavor,  exists
	// just letting it pass through un checked right now
	
	//create server
	vmname := fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	imageRef := d.ImageID
	flavorRef := d.Flavor
	userData
	//(**Openstack v2 Compute doesnt seem to support keypair injection)
	buildOpts := servers.CreateOpts{
		Name:       vmname,
		ImageRef:   imageRef,
		FlavorRef:  flavorRef,
		//KeyPair: keypair,
		UserData:   userData,
	}
	//create the server
  	s, sErr := servers.Create(client, buildOpts).Extract()
	log.Debugf("Err:", sErr)
	log.Infof("Creating server.")

	sWaitErr := servers.WaitForStatus(client, s.ID, "ACTIVE", 300)
	if sWaitErr {
		log.Debugf("Err:", sWaitErr)
		return sWaitErr
	}	
	log.Infof("Server created successfully.", s.ID)
	
	// *Warning only suitable for devstack
  	if d.NovaNetwork {
  	    addip, addIpErr := floatingips.AddNovaNetIp(client, addopts).Extract()
  	    if addIpErr {
		     log.Debugf("Err:", addIpErr)
	    }	
	    log.Infof("Adding Floating IP:", addip)
	
	    //create floating ip --nova-network? (compute vs neutron APIs) (**Dev effort for gophercloud APIs)
	    ipBuildOpts := floatingips.CreateNovaNetIpOpts{}
  	
  	    fip, floatErr := floatingips.CreateNovaNetIp(client, ipbuildopts).Extract()
  	    if floatErr {
		    log.Debugf("Err:", floatErr)
	    }	
	    log.Infof("Created Floating IP")
	    
	    instance := s.ID
	    //FixMe TODO, need to retreive IP from CreatNovaNetIp()
  	    ip := "192.168.1.225"
  	    pool := "public"
  	    addopts := floatingips.AddNovaNetIpOpts{
		    ServerID:    instance,
	    	IPAddress:   ip,
		    Pool:		 pool,
	    }
	
	    //Associate IP
	    addip, floatIpErr := floatingips.AddNovaNetIp(client, addopts).Extract()
	    if floatIpErr {
		    log.Debugf("Err:", floatIpErr)
	 	    return floatIpErr
	    }
	    //FixMe, TODO once we get IP from CreateNovaNetIP() we can 
	    // dynamically add this in
	    log.Infof("Adding Floating IP:", ip)
    } else{
    	//TODO Use Neutron Network related Commands
    	netClient := d.getNetworkClient()
    	
    	ipBuildOpts := floatingips.CreateOpts{
    		FloatingNetworkID:  d.FloatingIpNetwork
			FloatingIP          d.FloatingIpPort
    		}
    	
    	ip, sErr := floatingips.Create(client, ipBuildOpts).Extract()
		log.Debugf("Err:", sErr)
		log.Infof("Created Floating Ip",  ip)
   	}
	
	//set rules on security group for Docker Port, SSH, ICMP
	secErr := d.setSecurityGroups()
	if secErr {
		log.Infof("Error Setting up Security Group Ruless"}
	}

   return nil
}

func (d *Driver) setOpenstackVMName() {
	if d.OpenstackVMName == "" {
		d.OpenStackVMName = fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	}
}

func (d *Driver) getNetworkClient() *openstack.NewNetworkV2 {
   ident := 	d.IndentityEndpoint
   username := 	d.Username 
   password :=  d.Password
   tid := 		d.TenantID
   
   opts := gophercloud.AuthOptions{
 		 IdentityEndpoint: ident,
 		 Username: username,
		 Password: password,
 		 TenantID: tid,
		}
	// Authorize
	provider, err := openstack.AuthenticatedClient(opts)
	fmt.Println(reflect.TypeOf(provider), "Err:" , err)
	// Get the compute client
	netClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		    Name:   "neutron",
		    Region: "RegionOne",
		})
  	
  	return netClient
}

//provide the os-security-group rules for ICMP, SSH, and Docker 2357
func (d *Driver) setSecurityGroups() error {
	err := errors.New()
	client := d.getClient()
	
	secopts1 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	22,
		PortRangeMin:	22,
		SecGroupID:		"1",
	}
	secopts2 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	22,
		PortRangeMin:	22,
		SecGroupID:		"1",
	}
	s1, secErr1 := rules.Create(client, secopts1).Extract()
	fmt.Println("Err:", secErr1, s1)
	if secErr1 {
		return secErr1
	}
	s2, secErr2 := rules.Create(client, secopts2).Extract()
	fmt.Println("Err:", secErr2, s2)
	if secErr2 {
		return secErr2
	}


	secopts3 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	-1,
		PortRangeMin:	-1,
		SecGroupID:		"1",
	}
	secopts4 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	"-1",
		PortRangeMin:	"-1",
		SecGroupID:		"1",
	}
	s3, secErr3 := rules.Create(client, secopts3).Extract()
	fmt.Println("Err:", secErr3, s3)
	if secErr3 {
		return secErr4
	}
	s4, secErr4 := rules.Create(client, secopts4).Extract()
	fmt.Println("Err:", secErr4, s4)
	if secErr4 {
		return secErr4
	}
	
	secopts5 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:		"1",
	}
	secopts6 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:		"1",
	}
	s5, secErr5 := rules.Create(client, secopts5).Extract()
	fmt.Println("Err:", secErr5, s5)
	if secErr5 {
		return secErr5
	}
	s6, secErr6 := rules.Create(client, secopts6).Extract()
	fmt.Println("Err:", secErr6, s6)
	if secErr6 {
		return secErr6
	}
	
	return err
}
