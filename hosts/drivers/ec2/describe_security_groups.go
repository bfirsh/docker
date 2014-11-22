package ec2

type DescribeSecurityGroupsResponse struct {
	RequestId         string `xml"requestId"`
	SecurityGroupInfo []struct {
	} `xml:"securityGroupInfo>item"`
}
