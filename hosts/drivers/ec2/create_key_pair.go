package ec2

type CreateKeyPairResponse struct {
	KeyName        string `xml:"keyName"`
	KeyFingerprint string `xml:"keyFingerprint"`
	KeyMaterial    []byte `xml:"keyMaterial"`
}
