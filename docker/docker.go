package main

import (
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/client"
	"github.com/docker/docker/api/client/auth"
	"github.com/docker/docker/dockerversion"
	"github.com/docker/docker/hosts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/reexec"
	"github.com/docker/docker/utils"
)

func main() {
	if reexec.Init() {
		return
	}

	flag.Parse()
	// FIXME: validate daemon flags here

	if *flVersion {
		showVersion()
		return
	}

	if *flLogLevel != "" {
		lvl, err := log.ParseLevel(*flLogLevel)
		if err != nil {
			log.Fatalf("Unable to parse logging level: %s", *flLogLevel)
		}
		initLogging(lvl)
	} else {
		initLogging(log.InfoLevel)
	}

	// -D, --debug, -l/--log-level=debug processing
	// When/if -D is removed this block can be deleted
	if *flDebug {
		os.Setenv("DEBUG", "1")
		initLogging(log.DebugLevel)
	}

	// Backwards compatibility for deprecated --tls and --tlsverify options
	if *flTls || flag.IsSet("-tlsverify") {
		*flAuth = "cert"

		// Backwards compatibility for --tlscacert
		if *flCa != "" {
			*flAuthCa = *flCa
		}
		// Backwards compatibility for --tlscert
		if *flCert != "" {
			*flAuthCert = *flCert
		}
		// Backwards compatibility for --tlskey
		if *flKey != "" {
			*flAuthKey = *flKey
		}

		// Only verify against a CA if --tlsverify is set
		if !*flTlsVerify {
			*flAuthCa = ""
		}
	}

	if *flDaemon {
		if len(flHosts) == 0 {
			defaultHost := os.Getenv("DOCKER_HOST")
			if defaultHost == "" || *flDaemon {
				// If we do not have a host, default to unix socket
				defaultHost = fmt.Sprintf("unix://%s", api.DEFAULTUNIXSOCKET)
			}
			defaultHost, err := api.ValidateHostURL(defaultHost)
			if err != nil {
				log.Fatal(err)
			}
			flHosts = append(flHosts, defaultHost)
		}

		mainDaemon()
		return
	}

	var (
		host  *hosts.Host
		err   error
		store = hosts.NewStore()
	)

	trustKey, err := api.LoadOrCreateTrustKey(*flTrustKey)
	if err != nil {
		log.Fatal(err)
	}

	// Select active host if no host has been specified
	if len(flHosts) == 0 {
		host, err = store.GetActive()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		if len(flHosts) > 1 {
			log.Fatal("Please specify only one -H")
		}

		hostURL := flHosts[0]

		// Attempt to find a host if it's a valid name
		if _, err := hosts.ValidateHostName(hostURL); err == nil {
			exists, err := store.Exists(hostURL)
			if err != nil {
				log.Fatal(err)
			}
			if !exists {
				log.Fatal(fmt.Errorf("Host %q does not exist. Create it using 'docker hosts create'.", hostURL))
			}
			host, err = store.Load(hostURL)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			host = hosts.NewDefaultHost(hostURL)
		}
	}

	// Select an auth method
	switch *flAuth {
	// By default, use no auth for default host, cert auth for normal hosts
	case "":
		if !host.IsDefault() {
			host.AuthMethod = &auth.IdentityAuth{
				TrustKey:       trustKey,
				KnownHostsPath: *flTrustHosts,
			}
		}
	// Override auth method
	case "cert":
		host.AuthMethod = &auth.CertAuth{
			CAPath:   *flAuthCa,
			CertPath: *flAuthCert,
			KeyPath:  *flAuthKey,
		}
	case "identity":
		host.AuthMethod = &auth.IdentityAuth{
			TrustKey:       trustKey,
			KnownHostsPath: *flTrustHosts,
		}
	case "none":
		host.AuthMethod = nil
	default:
		log.Fatalf("Unknown auth method: %s", *flAuth)
	}

	cli := client.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, trustKey, host)

	if err := cli.Cmd(flag.Args()...); err != nil {
		if sterr, ok := err.(*utils.StatusError); ok {
			if sterr.Status != "" {
				log.Println(sterr.Status)
			}
			os.Exit(sterr.StatusCode)
		}
		log.Fatal(err)
	}
}

func showVersion() {
	fmt.Printf("Docker version %s, build %s\n", dockerversion.VERSION, dockerversion.GITCOMMIT)
}
