package ec2

type DescribeInstancesResponse struct {
	RequestId      string `xml:"requestId"`
	ReservationSet []struct {
		InstancesSet []struct {
			InstanceId    string `xml:"instanceId"`
			ImageId       string `xml:"imageId"`
			InstanceState struct {
				Code int    `xml:"code"`
				Name string `xml:"name"`
			} `xml:"instanceState"`
			NetworkInterfaceSet []struct {
				Association struct {
					PublicIp      string `xml:"publicIp"`
					PublicDnsName string `xml:"publicDnsName"`
					IpOwnerId     string `xml:"ipOwnerId"`
				} `xml:"association"`
			} `xml:"networkInterfaceSet>item"`
		} `xml:"instancesSet>item"`
	} `xml:"reservationSet>item"`
}
