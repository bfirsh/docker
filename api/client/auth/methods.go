package auth

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/libtrust"
)

// AuthMethod lazily creates tls.Config objects given a protocol and address
type AuthMethod interface {
	TLSConfig(proto, addr string) (*tls.Config, error)
}

// IdentityAuth is an auth method for libtrust identity
type IdentityAuth struct {
	TrustKey       libtrust.PrivateKey
	KnownHostsPath string
}

func (a *IdentityAuth) TLSConfig(proto, addr string) (*tls.Config, error) {
	tlsConfig := createTLSConfig()

	// Load known hosts
	knownHosts, err := libtrust.LoadKeySetFile(a.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("Could not load trusted hosts file: %s", err)
	}

	// Generate CA pool from known hosts
	allowedHosts, err := libtrust.FilterByHosts(knownHosts, addr, false)
	if err != nil {
		return nil, fmt.Errorf("Error filtering hosts: %s", err)
	}
	certPool, err := libtrust.GenerateCACertPool(a.TrustKey, allowedHosts)
	if err != nil {
		return nil, fmt.Errorf("Could not create CA pool: %s", err)
	}
	tlsConfig.ServerName = "docker"
	tlsConfig.RootCAs = certPool

	// Generate client cert from trust key
	x509Cert, err := libtrust.GenerateSelfSignedClientCert(a.TrustKey)
	if err != nil {
		return nil, fmt.Errorf("Certificate generation error: %s", err)
	}
	tlsConfig.Certificates = []tls.Certificate{{
		Certificate: [][]byte{x509Cert.Raw},
		PrivateKey:  a.TrustKey.CryptoPrivateKey(),
		Leaf:        x509Cert,
	}}

	// Connect to server to see if it is a known host
	tlsConfig.InsecureSkipVerify = true
	testConn, err := tls.Dial(proto, addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("TLS Handshake error: %s", err)
	}
	opts := x509.VerifyOptions{
		Roots:         tlsConfig.RootCAs,
		CurrentTime:   time.Now(),
		DNSName:       tlsConfig.ServerName,
		Intermediates: x509.NewCertPool(),
	}

	certs := testConn.ConnectionState().PeerCertificates
	for i, cert := range certs {
		if i == 0 {
			continue
		}
		opts.Intermediates.AddCert(cert)
	}
	_, err = certs[0].Verify(opts)
	if err != nil {
		if _, ok := err.(x509.UnknownAuthorityError); ok {
			pubKey, err := libtrust.FromCryptoPublicKey(certs[0].PublicKey)
			if err != nil {
				return nil, fmt.Errorf("Error extracting public key from certificate: %s", err)
			}

			// If server is not a known host, prompt user to ask whether it should
			// be trusted and add to the known hosts file
			if promptUnknownKey(pubKey, addr) {
				pubKey.AddExtendedField("hosts", []string{addr})
				err = libtrust.AddKeySetFile(a.KnownHostsPath, pubKey)
				if err != nil {
					return nil, fmt.Errorf("Error saving updated host keys file: %s", err)
				}

				ca, err := libtrust.GenerateCACert(a.TrustKey, pubKey)
				if err != nil {
					return nil, fmt.Errorf("Error generating CA: %s", err)
				}
				tlsConfig.RootCAs.AddCert(ca)
			} else {
				return nil, fmt.Errorf("Cancelling request due to invalid certificate")
			}
		} else {
			return nil, fmt.Errorf("TLS verification error: %s", err)
		}
	}

	testConn.Close()
	tlsConfig.InsecureSkipVerify = false

	return tlsConfig, nil
}

// CertAuth is an auth method that uses TLS certificates
type CertAuth struct {
	CAPath   string
	CertPath string
	KeyPath  string
}

func (a *CertAuth) TLSConfig(proto, addr string) (*tls.Config, error) {
	tlsConfig := createTLSConfig()

	// Verify the server against a CA certificate?
	if a.CAPath != "" {
		certPool := x509.NewCertPool()
		file, err := ioutil.ReadFile(a.CAPath)
		if err != nil {
			return nil, fmt.Errorf("Couldn't read ca cert %s: %s", a.CAPath, err)
		}
		certPool.AppendCertsFromPEM(file)
		tlsConfig.RootCAs = certPool
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	// Try to load and send client certificates
	if a.CertPath != "" && a.KeyPath != "" {
		_, errCert := os.Stat(a.CertPath)
		_, errKey := os.Stat(a.KeyPath)
		if errCert == nil && errKey == nil {
			cert, err := tls.LoadX509KeyPair(a.CertPath, a.KeyPath)
			if err != nil {
				return nil, fmt.Errorf("Couldn't load X509 key pair: %s. Key encrypted?", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}
	return tlsConfig, nil
}

// createTLSConfig creates the base tls.Config used by auth methods with some
// sensible defaults
func createTLSConfig() *tls.Config {
	return &tls.Config{
		// Avoid fallback to SSL protocols < TLS1.0
		MinVersion: tls.VersionTLS10,
	}
}

func promptUnknownKey(key libtrust.PublicKey, host string) bool {
	fmt.Printf("The authenticity of host %q can't be established.\nRemote key ID %s\n", host, key.KeyID())
	fmt.Printf("Are you sure you want to continue connecting (yes/no)? ")
	reader := bufio.NewReader(os.Stdin)
	line, _, err := reader.ReadLine()
	if err != nil {
		log.Fatalf("Error reading input: %s", err)
	}
	input := strings.TrimSpace(strings.ToLower(string(line)))
	return input == "yes" || input == "y"
}
