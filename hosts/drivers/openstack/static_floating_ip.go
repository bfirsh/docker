package main

import (
	"fmt"
	"reflect"
	//"strings"
	//"github.com/docker/docker/utils"
	//"encoding/json"
	gophercloud "github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	//"github.com/rackspace/gophercloud/pagination"
    //"github.com/rackspace/gophercloud/openstack/compute/v2/flavors"
    //"github.com/rackspace/gophercloud/openstack/compute/v2/images"
    //"github.com/rackspace/gophercloud/openstack/compute/v2/servers"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/ports"
    "github.com/rackspace/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/groups"
    //"github.com/rackspace/gophercloud/openstack/networking/v2/extensions/security/rules"
)

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
  	
  	ipbuildopts := floatingips.CreateNovaNetIpOpts{}
  	
  	s, err := floatingips.CreateNovaNetIp(client, ipbuildopts).Extract()
	fmt.Println("Err:", err)
	fmt.Println("Creating Floating IP")
	fmt.Println(s)
	fmt.Println(reflect.TypeOf(s), "Err:" , err)
  	
  	ips, errr := floatingips.GetNovaNetIps(client).Extract()
  	fmt.Println(errr)
  	fmt.Println(ips)
  	
  	instance := "52ed5150-c645-4951-8f8e-e7042284f728"
  	ip := "192.168.1.225"
  	addopts := floatingips.AddNovaNetIpOpts{
		ServerID:    instance,
		IPAddress:   ip,
		Pool:		 "public",
	}
  	
  	addip, err4 := floatingips.AddNovaNetIp(client, addopts).Extract()
	fmt.Println("Err:", err4)
	fmt.Println("Adding Floating IP:", addip)
  	
}  	