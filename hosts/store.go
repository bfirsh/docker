package hosts

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
)

// Store persists hosts on the filesystem
type Store struct {
	Path string
}

func NewStore() *Store {
	rootPath := path.Join(os.Getenv("HOME"), ".docker/hosts")
	return &Store{Path: rootPath}
}

func (s *Store) Create(name string, driverName string, driverOptions map[string]string) (*Host, error) {
	hostPath := path.Join(s.Path, name)

	if _, err := os.Stat(hostPath); err == nil {
		return nil, fmt.Errorf("Host %q already exists", name)
	}

	host, err := NewHost(name, driverName, driverOptions, hostPath)
	if err != nil {
		return host, err
	}
	if err := host.Create(); err != nil {
		return host, err
	}
	return host, nil
}

func (s *Store) Remove(name string) error {
	host, err := s.Load(name)
	if err != nil {
		return err
	}
	return host.Remove()
}

func (s *Store) List() ([]Host, error) {
	dir, err := ioutil.ReadDir(s.Path)
	if err != nil {
		return nil, err
	}
	var hosts []Host
	for _, file := range dir {
		if file.IsDir() {
			host, err := s.Load(file.Name())
			if err != nil {
				return nil, err
			}
			hosts = append(hosts, *host)
		}
	}
	return hosts, nil
}

func (s *Store) Exists(name string) (bool, error) {
	_, err := os.Stat(path.Join(s.Path, name))
	if os.IsNotExist(err) {
		return false, nil
	} else if err == nil {
		return true, nil
	}
	return false, err
}

func (s *Store) Load(name string) (*Host, error) {
	hostPath := path.Join(s.Path, name)
	return LoadHost(name, hostPath)
}
