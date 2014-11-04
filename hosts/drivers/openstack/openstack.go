package openstack

// *Fixme
// Temporarily using gophercloud from rackspace.
// this is just code bloat right now, will get functionality
// of openstack needed into a seperate smaller library
import (
	"fmt"
	"os/exec"
	"errors"

	"github.com/docker/docker/pkg/log"
	//"github.com/docker/docker/hosts/ssh"
	"github.com/docker/docker/hosts/state"
	"github.com/docker/docker/hosts/drivers"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/utils"
	gophercloud "github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/pagination"
	//"github.com/rackspace/gophercloud/openstack/compute/v2/flavors"
        //"github.com/rackspace/gophercloud/openstack/compute/v2/images"
	"github.com/rackspace/gophercloud/openstack/compute/v2/servers"
	"github.com/rackspace/gophercloud/openstack/networking/v2/ports"
	"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/rules"
)

type Driver struct {
	IdentityEndpoint  string
	KeyPair		      string
	AvailabilityZone  string
	UserUUID	      int
	Username	      string
	Password      	  string
	TenantID	      string
	TenantName 	      string
	RegionID     	  string 
	RegionName        string	
	OpenstackVMID     int
	OpenstackVMName   string
	ImageID       	  string
	IPAddress   	  string
	Flavor        	  string
	FloatingIpNetwork string
	FloatingIpPort	  string
    NetworkID	      string
	SecurityGroup     string
	NovaNetwork  	  bool
	NameServer  	  string
	storePath   	  string
}

type CreateFlags struct {
	IdentityEndpoint  *string
	KeyPair		      *string
	Username	      *string
	Password      	  *string
	ImageID		      *string
	TenantID	      *string
    RegionName	      *string
	Flavor       	  *string
	FloatingIpNetwork *string
	FloatingIpPort	  *string
	NetworkID	      *string
	SecurityGroup	  *string
	NovaNetwork	      *bool
	NameServer	      *string
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
	createFlags.KeyPair = cmd.String(
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
        createFlags.RegionName = cmd.String(
                []string{"-openstack-region-name"},
                "RegionOne",
                "Openstack Region",
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
        createFlags.NetworkID = cmd.String(
                []string{"-openstack-net-id"},
                "",
                "Openstack Network UUID",
       ) 
	createFlags.SecurityGroup = cmd.String(
		[]string{"-openstack-secgroup-id"},
		"",
		"Openstack Security Group Setting",
	)
	createFlags.NovaNetwork = cmd.Bool(
		[]string{"-openstack-nova-net"},
		false,
		"Using Openstack Nova Network?",
	)
        createFlags.NameServer = cmd.String(
                []string{"-openstack-nameserver"},
                "",
                "Using Seperate Openstack NameServer",
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
	d.KeyPair = *flags.KeyPair
	d.Username = *flags.Username
	d.Password = *flags.Password
	d.ImageID =  *flags.ImageID
	d.TenantID = *flags.TenantID
    d.RegionName = *flags.RegionName
	d.Flavor = *flags.Flavor
	d.FloatingIpNetwork = *flags.FloatingIpNetwork
	d.NetworkID = *flags.NetworkID
	d.SecurityGroup = *flags.SecurityGroup
	d.NovaNetwork = *flags.NovaNetwork
	d.NameServer = *flags.NameServer
	
	// *Fixme, think about adding the function
	// pts, err := openstack.AuthOptionsFromEnv()
	// from gophercloud that check for auth in the
	// environment.
	
	if d.IdentityEndpoint == "" {
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
        if d.SecurityGroup == "" {
                return fmt.Errorf("openstack driver requires the --openstack-secgroup-id option")
        }
	// Flavor is defaulted to m1.small
	// FloatingIpNetwork defaulted to public
        // NovaNetwork defaulted to false
        if d.NovaNetwork {
        	log.Infof("Using Nova Network Config")
        } else {
        	if d.FloatingIpNetwork == "" {
	   		return fmt.Errorf("openstack driver requires the --openstack-floating-net option")
	  	}
                if d.NetworkID == "" {
                        return fmt.Errorf("openstack driver requires the --openstack-net-id option")
                }
        }
    
	return nil
}

func (d *Driver) Create() error {
	d.setOpenstackVMName()
	
	// *FixMe need to ingest keypair from openstack
	//Get SSH key from flags, or create one.
	
	//Runn cloud-init scripts instead of ssh commands
	//Load User Data for docker installation OR wait for SSH, 
    var cloudInitData []byte
    if d.NameServer == "" {
	  cloudInitData = []byte(""+
	  "#!/bin/bash\n"+
	  "sudo echo -e 'docker\ndocker' | passwd root\n" +
	  "sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9\n"+
	  "sudo sh -c 'echo deb https://get.docker.com/ubuntu docker main > /etc/apt/sources.list.d/docker.list'\n" +
	  "sudo apt-get update\n" +
	  "sudo apt-get -y install lxc-docker\n" +
	  "sudo service docker stop\n" +
	  "sudo service ufw stop\n" +
	  "sudo docker -d -H tcp://0.0.0.0:2375 &\n")
	} else {
		  //Support different nameserver injection
          cloudInitData = []byte(""+
          "#!/bin/bash\n"+
          "sudo echo -e 'docker\ndocker' | passwd root\n" +
          "sudo echo 'nameserver '" +d.NameServer+" > /etc/resolv.conf\n"+
          "sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9\n"+
          "sudo sh -c 'echo deb https://get.docker.com/ubuntu docker main > /etc/apt/sources.list.d/docker.list'\n" +
          "sudo apt-get update\n" +
          "sudo apt-get -y install lxc-docker\n" +
          "sudo service docker stop\n" +
          "sudo service ufw stop\n" +
          "sudo docker -d -H tcp://0.0.0.0:2375 &\n")
        }
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
	userData := cloudInitData 
	keypair := d.KayPair

	buildOpts := servers.CreateOpts{
		Name:       vmname,
		ImageRef:   imageRef,
		FlavorRef:  flavorRef,
		KeyPair:    keypair,
		UserData:   userData,
	}
        if !d.NovaNetwork {
           var network servers.Network
           network.UUID = d.NetworkID
           var networks []servers.Network
           networks = append(networks, network)
           buildOpts.Networks = networks
        }
        
	//Create the server
  	s, sErr := servers.Create(client, buildOpts).Extract()
        if sErr != nil {
           log.Infof("Error Creating Server", sErr)
        }
	log.Debugf("Err:", sErr)
	log.Infof("Creating server.")

	sWaitErr := servers.WaitForStatus(client, s.ID, "ACTIVE", 300)
	if sWaitErr != nil {
		log.Debugf("Err:", sWaitErr)
		return sWaitErr
	}	
	log.Infof("Server created successfully.", s.ID)
	
	// ***Warning only suitable for devstack
	//*FixMe (Experimental) hardcoded IP for FLOATING_IP_POLL 1st address
  	if d.NovaNetwork {
	    
	    // create floating ip --nova-network? (compute vs neutron APIs)
	    // (**Added to gophercloud APIs)
	    ipBuildOpts := floatingips.CreateNovaNetIpOpts{}
  	
  	    fip, floatErr := floatingips.CreateNovaNetIp(client, ipBuildOpts).Extract()
  	    if floatErr != nil {
		    log.Debugf("Err:", floatErr)
	    }	
	    log.Infof("Created Floating IP", fip)
	    
	    instance := s.ID
	    
	    //FixMe TODO, need to retreive IP from CreatNovaNetIp()
	    //*Need to create "CreateNovaNetIpResult.go" in requests.go in gophercloud
  	    ip := "192.168.1.225"
  	    pool := "public"
  	    addopts := floatingips.AddNovaNetIpOpts{
		    ServerID:    instance,
	    	    IPAddress:   ip,
		    Pool:	 pool,
	    }
	
	    //Associate IP
	    addip, floatIpErr := floatingips.AddNovaNetIp(client, addopts).Extract()
	    if floatIpErr != nil  {
		    log.Debugf("Err:", floatIpErr)
	 	    return floatIpErr
	    }
            log.Infof("AddedNovaIP: ", addip)

	    //FixMe, TODO once we get IP from CreateNovaNetIP() we can 
	    // dynamically add this in
	    log.Infof("Adding Floating IP:", ip)
	    d.IPAddress = ip    
    } else{
    	
    	// Support OpenStack Neutron
    	netClient := d.getNetworkClient()
    	
    	ip, err := d.getIpFromVmId(s.ID, vmname)
    	if err != nil { log.Infof("Couldn't Find IPAddress") }
    	portID, portErr := d.getPortIdFromIp(ip, d.TenantID)
    	if portErr != nil { log.Infof("Couldn't Find Port") }
    	
    	// FixMe! ips are created, but don't reuse ones that
    	// are already allocated. 
    	ipBuildOpts := floatingips.CreateOpts{
	    	FloatingNetworkID:  d.FloatingIpNetwork,
			PortID:             portID,
    		}
    	
    	fip, ipErr := floatingips.Create(netClient, ipBuildOpts).Extract()
    	if ipErr != nil {
		    log.Debugf("Err:", ipErr)
	 	    return ipErr
	    }
		log.Infof("Created Floating Ip",  fip.FloatingIP)
		d.IPAddress = fip.FloatingIP
   	}
	
	//set rules on security group for Docker Port, SSH, ICMP
	// FixMe* you may see errors on these request if they
	// already exists. neeeds error handeling
	secErr := d.setSecurityGroups()
	if secErr != nil {
		log.Infof("Error Setting up Security Group Rules")
	}

   return nil
}

//**
//FixMe (Clean this up and make sure it works against other Openstack Neutron Instances)
//**
func (d *Driver) getIpFromVmId(id string, name string) (string, error) {
        client := d.getClient()
	opts := servers.ListOpts{Name: name}
	pager := servers.List(client, opts)
	
	var ip string = ""
	log.Debugf("Looking for ", id, "'s ip")
	pErr := pager.EachPage(func(page pagination.Page) (bool, error) {
        serverList, err := servers.ExtractServers(page)
        log.Debugf("Err:" , err)
          for _, s := range serverList {
                // We can get status this way
                // FixMe! s.Status (Add to geState func)
                addresses := s.Addresses
		for _, ipAdd := range addresses {
                        ipAddMap := ipAdd.([]interface{})
                	for k, v := range ipAddMap {
                          log.Debugf("KEY: ", k, "VALUE: ", v)
                          switch vv := v.(type) {
                          case string:
     				log.Debugf("String... Pass")
   			  case map[string]interface{}:
        		   	log.Debugf("Is a map...keep looking")
                                for i, u := range vv {
                                        if i == "addr" {
                                          log.Debugf("FOUND IP!: ", u)
                                          ip = u.(string)
                                        }
                                }
  			  default:
      			 	log.Debugf("I don't know how to handle this type")
		    	  }
                         }
                     }
          }
          return true, nil
        })
        log.Debugf("Paging Err:" , pErr)
        return ip, nil
}

// Retrieve the PortID for floating ip association from the IP/Tenant
func (d *Driver) getPortIdFromIp(ip string, tenantId string) (string, error) {
        client := d.getNetworkClient()
	opts := ports.ListOpts{TenantID: tenantId}
	pager := ports.List(client, opts)	
        var portId string = ""	
        pErr := pager.EachPage(func(page pagination.Page) (bool, error) {
        portList, err := ports.ExtractPorts(page)
        log.Debugf("Err:" , err)
          for _, p := range portList {
                ipAddresses := p.FixedIPs
                for _, ipAdd := range ipAddresses {
                	if ipAdd.IPAddress == ip {
                		portId = p.ID
                		break
                	}
                }
          }
          return true, nil
        })
        log.Debugf("Paging Err:" , pErr)
    return portId, nil
}


func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tcp://%s:2375", ip), nil
}

func (d *Driver) GetIP() (string, error) {
	return d.IPAddress, nil
}

func (d *Driver) setOpenstackVMName() {
	if d.OpenstackVMName == "" {
		d.OpenstackVMName = fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	}
}

//TODO
func (d *Driver) GetState() (state.State, error) {
	//FixMe!
	return state.Running, nil
}

//TODO
func (d *Driver) Start() error {
	return nil
}
//TODO
func (d *Driver) Stop() error {
	return nil
}
//TODO
func (d *Driver) Remove() error {
	return nil
}
//TODO
func (d *Driver) Restart() error {
	return nil
}
//TODO
func (d *Driver) Kill() error {
	return nil
}
//TODO
func (d *Driver) GetSSHCommand(args ...string) *exec.Cmd {
	//*FixMe need to import SSH Key specifed / or create one and import it.
	//return ssh.GetSSHCommand(d.IPAddress, 22, "root", d.sshKeyPath(), args...)
	return nil
}


func (d *Driver) getNetworkClient() *gophercloud.ServiceClient {
	
   // why did i set these vars first?
   // Go newb	
   ident := 	d.IdentityEndpoint
   username := 	d.Username 
   password :=  d.Password
   tid := 	d.TenantID
   
   opts := gophercloud.AuthOptions{
 		 IdentityEndpoint: ident,
 		 Username: username,
		 Password: password,
 		 TenantID: tid,
		}
	// Authorize
	provider, err := openstack.AuthenticatedClient(opts)
        if err != nil {
		log.Debugf("Err:" , err)
        }
	// Get the compute client
	netClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		    Name:   "neutron",
		    Region: d.RegionName,
		})
  	
  	return netClient
}

func (d *Driver) getClient() *gophercloud.ServiceClient {
	// why did i set these vars first?
    // Go newb
   	ident :=     d.IdentityEndpoint
   	username :=  d.Username
   	password :=  d.Password
   	tid :=       d.TenantID

   	opts := gophercloud.AuthOptions{
                 IdentityEndpoint: ident,
                 Username: username,
                 Password: password,
                 TenantID: tid,
                }
        // Authorize
        provider, err := openstack.AuthenticatedClient(opts)
        if err != nil {
        	log.Debugf("Err:" , err)
        }
        // Get the compute client
        client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
                    Region: d.RegionName,
                })

       return client
}

//provide the os-security-group rules for ICMP, SSH, and Docker 2357
func (d *Driver) setSecurityGroups() error {
	err := errors.New("")
	client := d.getNetworkClient()
	
	secopts1 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:	rules.Ether4,
		Protocol: 	rules.ProtocolICMP,
		PortRangeMax:	"22",
		PortRangeMin:	"22",
		SecGroupID:	d.SecurityGroup,
	}
	secopts2 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:	rules.Ether4,
		Protocol: 	rules.ProtocolTCP,
		PortRangeMax:	"22",
		PortRangeMin:	"22",
		SecGroupID:	d.SecurityGroup,
	}
	s1, secErr1 := rules.Create(client, secopts1).Extract()
	fmt.Println("Err:", secErr1, s1)
	if secErr1 != nil {
		//log.Errorf(secErr1)
                fmt.Println(secErr1)
	}
	s2, secErr2 := rules.Create(client, secopts2).Extract()
	fmt.Println("Err:", secErr2, s2)
	if secErr2 != nil {
		//log.Errorf(secErr2)
                fmt.Println(secErr2)
	}

	secopts3 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:	rules.Ether4,
		Protocol: 	rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:	d.SecurityGroup,
	}
	secopts4 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:	rules.Ether4,
		Protocol: 	rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:	d.SecurityGroup,
	}
	s3, secErr3 := rules.Create(client, secopts3).Extract()
	fmt.Println("Err:", secErr3, s3)
	if secErr3 != nil {
		//log.Errorf(secErr3)
                fmt.Println(secErr3)
	}
	s4, secErr4 := rules.Create(client, secopts4).Extract()
	fmt.Println("Err:", secErr4, s4)
	if secErr4 != nil {
		//log.Errorf(secErr4)
                fmt.Println(secErr4)
	}
	
	return err
}
