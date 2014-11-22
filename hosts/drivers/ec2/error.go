package ec2

type ErrorResponse struct {
	Errors []struct {
		Code    string
		Message string
	} `xml:"Errors>Error"`
	RequestID string
}
