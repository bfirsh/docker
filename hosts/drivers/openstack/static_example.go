package main

import (
	"fmt"
	"reflect"
	"strings"
	"github.com/docker/docker/utils"
	gophercloud "github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/pagination"
    "github.com/rackspace/gophercloud/openstack/compute/v2/flavors"
    "github.com/rackspace/gophercloud/openstack/compute/v2/images"
    "github.com/rackspace/gophercloud/openstack/compute/v2/servers"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/ports"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/groups"
    "github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/rules"
)

//Showcases the main workflow of how the docker-host-managment
//works with the openstack APIs from rackspace to setup a docker host.
func main() {

    // Authentication (Passed in as flags usually)
    username := "admin"
    password := "admin"
    tenant_id := "eb2a911fb4f0490e93dbdd1bb807f2cf"

	opts := gophercloud.AuthOptions{
 	 IdentityEndpoint: "http://192.168.1.13:5000/v2.0",
 	 Username: username,
	 Password: password,
 	 TenantID: tenant_id,
	}
	
	// Authorize
	provider, err := openstack.AuthenticatedClient(opts)
	fmt.Println(reflect.TypeOf(provider), "Err:" , err)
	
	// Get the compute client
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
  		Region: "RegionOne",})
  		
  	fmt.Println(reflect.TypeOf(client), "Err:" , err)	
  	
  	// Make sure the flavor exists
  	fopts := flavors.ListOpts{}
  	flavor, err := flavors.Get(client, "3").Extract()
  	fmt.Println(flavor)
  	pager := flavors.ListDetail(client, fopts)
  	
  	// iterate on the pages for list flavors
  	errr := pager.EachPage(func(page pagination.Page) (bool, error) {
      flavorList, err := flavors.ExtractFlavors(page)
      fmt.Println("Err:" , err)
      for _, f := range flavorList {
       // "f" will be a flavors.Flavor
       fmt.Println(f)
       if strings.Contains(f.Name, "m1.small") {
       	fmt.Println("Found Flavor")
       }
      }
      return true, nil
    })
  	fmt.Println("Err:" , errr)
  	
  	// Get back a images.Image struct
  	// You will need to use a Ubuntu image for v1 of this
  	// we will use cloud init to run apt-get based commands.
  	// in the future we can detect this early looking at the
  	// image or by using the userdata
  	image_uuid := "d6a498cf-0447-4469-913d-0dedd4705004"
    image, err := images.Get(client, image_uuid).Extract()
    fmt.Println(image.Name, "Err:", err)
  	
  	//Servers
	sopts := servers.ListOpts{}

	// Retrieve a pager (i.e. a paginated collection)
	spager := servers.List(client, sopts)

	// Define an anonymous function to be executed on each page's iteration
	errrr := spager.EachPage(func(page pagination.Page) (bool, error) {
	  serverList, err := servers.ExtractServers(page)
       fmt.Println("Err:" , err)
	  for _, s := range serverList {
		fmt.Println(s)
	  }
	  return true, nil
	})
	fmt.Println("Err:" , errrr)
	
	
	// create the server
	sname := fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	imageref := image_uuid
	//keypair := "keypair1"
	flavorref := "11"
	udata := []byte(""+
	"#!/bin/bash\n"+
	"sudo echo -e 'docker\ndocker' | passwd root\n" +
	"sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9\n"+
	"sudo sh -c 'echo deb https://get.docker.com/ubuntu docker main " +
	"> /etc/apt/sources.list.d/docker.list'\n" +
	"sudo apt-get update\n" +
	"sudo apt-get install lxc-docker\n" +
	"sudo service lxc-docker stop\n" +
	"sudo service ufw stop\n" +
	"sudo docker -d -H tcp://0.0.0.0:2375 &\n")
	//ssecgroups := []string{"default"}
  	sbuildopts := servers.CreateOpts{
		Name:       sname,
		ImageRef:   imageref,
		FlavorRef:  flavorref,
		//SecurityGroups:    ssecgroups,
		//KeyPair: keypair,
		UserData:   udata,
	}
  	
  	//create the server
  	s, err4 := servers.Create(client, sbuildopts).Extract()
	fmt.Println("Err:", err4)
	fmt.Println("Creating server.")

	err5 := servers.WaitForStatus(client, s.ID, "ACTIVE", 300)
	fmt.Println("Err:", err5)
	fmt.Println("Server created successfully.", s.ID)
	
	//TODO Test Floating IPs and Ports with Compute/Neutron APIs
		//- Nova APIS (DONE) see statis_floating_example.go
		//- Neutron TODO
		
	//neutronClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
    //	Name:   "neutron",
    //	Region: "RegionOne",
	//})	
		
	//TODO Test Security Groups and Rules for SSH, ICMP and Docker
	secopts1 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	22,
		PortRangeMin:	22,
		SecGroupID:		"1",
	}
	secopts12 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	22,
		PortRangeMin:	22,
		SecGroupID:		"1",
	}
	s1, err7 := rules.Create(client, secopts1).Extract()
	fmt.Println("Err:", err7, s1)
	s2, err8 := rules.Create(client, secopts12).Extract()
	fmt.Println("Err:", err8, s2)


	secopts2 := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	-1,
		PortRangeMin:	-1,
		SecGroupID:		"1",
	}
	secopts21 := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolICMP,
		PortRangeMax:	"-1",
		PortRangeMin:	"-1",
		SecGroupID:		"1",
	}
	s3, err9 := rules.Create(client, secopts2).Extract()
	fmt.Println("Err:", err9, s3)
	s4, err10 := rules.Create(client, secopts21).Extract()
	fmt.Println("Err:", err10, s4)
	
	secoptsdockerin := rules.CreateOpts{
		Direction:      rules.DirIngress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:		"1",
	}
	secoptsdockerout := rules.CreateOpts{
		Direction:      rules.DirEgress,
		EtherType:		rules.Ether4,
		Protocol: 		rules.ProtocolTCP,
		PortRangeMax:	"2375",
		PortRangeMin:	"2375",
		SecGroupID:		"1",
	}
	s5, err11 := rules.Create(client, secoptsdockerin).Extract()
	fmt.Println("Err:", err11, s5)
	s6, err12 := rules.Create(client, secoptsdockerout).Extract()
	fmt.Println("Err:", err12, s6)
	
}       