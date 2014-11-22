package ec2

type RunInstancesResponse struct {
	RequestId     string        `xml:"requestId"`
	ReservationId string        `xml:"reservationId"`
	OwnerId       string        `xml:"ownerId"`
	Instances     []Ec2Instance `xml:"instancesSet>item"`
}

type Ec2Instance struct {
	InstanceId string `xml:"instanceId"`
	ImageId    string `xml:"imageId"`
	State      struct {
		Code int    `xml:"code"`
		Name string `xml:"name"`
	} `xml:"instanceState"`
	PrivateDnsName string `xml:"privateDnsName"`
	DnsName        string `xml:"dnsName"`
	Reason         string `xml:"reason"`
	AmiLaunchIndex string `xml:"amiLaunchIndex"`
	ProductCodes   string `xml:"productCodes"`
	InstanceType   string `xml:"instanceType"`
	LaunchTime     string `xml:"launchTime"`
	Placement      struct {
		AvailabilityZone string `xml:"availabilityZone"`
		GroupName        string `xml:"groupName"`
		Tenancy          string `xml:"tenancy"`
	} `xml:"placement"`
	KernelId   string `xml:"kernelId"`
	Monitoring struct {
		State string `xml:"state"`
	} `xml:"monitoring"`
	SubnetId         string `xml:"subnetId"`
	VpcId            string `xml:"vpcId"`
	PrivateIpAddress string `xml:"privateIpAddress"`
	SourceDestCheck  bool   `xml:"sourceDestCheck"`
	GroupSet         []struct {
		GroupId   string `xml:"groupId"`
		GroupName string `xml:"groupName"`
	} `xml:"groupSet"`
	StateReason struct {
		Code    string `xml:"code"`
		Message string `xml:"message"`
	} `xml:"stateReason"`
	Architecture        string `xml:"architecture"`
	RootDeviceType      string `xml:"rootDeviceType"`
	RootDeviceName      string `xml:"rootDeviceName"`
	BlockDeviceMapping  string `xml:"blockDeviceMapping"`
	VirtualizationType  string `xml:"virtualizationType"`
	ClientToken         string `xml:"clientToken"`
	Hypervisor          string `xml:"hypervisor"`
	NetworkInterfaceSet []struct {
		NetworkInterfaceId string `xml:"networkInterfaceId"`
		SubnetId           string `xml:"subnetId"`
		VpcId              string `xml:"vpcId"`
		Description        string `xml:"description"`
		OwnerId            string `xml:"ownerId"`
		Status             string `xml:"status"`
		MacAddress         string `xml:"macAddress"`
		PrivateIpAddress   string `xml:"privateIpAddress"`
		PrivateDnsName     string `xml:"privateDnsName"`
		SourceDestCheck    string `xml:"sourceDestCheck"`
		GroupSet           []struct {
			GroupId   string `xml:"groupId"`
			GroupName string `xml:"groupName"`
		} `xml:"groupSet>item"`
		Attachment struct {
			AttachmentId        string `xml:"attachmentId"`
			DeviceIndex         string `xml:"deviceIndex"`
			Status              string `xml:"status"`
			AttachTime          string `xml:"attachTime"`
			DeleteOnTermination bool   `xml:"deleteOnTermination"`
		} `xml:"attachment"`
		PrivateIpAddressesSet []struct {
			PrivateIpAddress string `xml:"privateIpAddress"`
			PrivateDnsName   string `xml:"privateDnsName"`
			Primary          bool   `xml:"primary"`
		} `xml:"privateIpAddressesSet>item"`
	} `xml:"networkInterfaceSet>item"`
	EbsOptimized bool `xml:"ebsOptimized"`
}
