package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/calavera/dkvolume"
)

type volume struct {
	name        string
	device		string
	connections int
}

type cephDriver struct {
	root       	string
	pool		string
	volumes    	map[string]*volume
	m          	sync.Mutex
}

func newCephDriver(root, pool string) cephDriver {
	d := cephDriver{
		root:   root,
		pool:	pool,
		volumes: map[string]*volume{},
	}
	return d
}

func (d cephDriver) Create(r dkvolume.Request) dkvolume.Response {
	log.Printf("Create(%s)\n", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	m := d.mountpoint(r.Name)

	if _, ok := d.volumes[m]; ok {
		return dkvolume.Response{}
	}

	// TODO Actually create an RBD

	return dkvolume.Response{}
}

func (d cephDriver) Remove(r dkvolume.Request) dkvolume.Response {
	log.Printf("Removing volume %s\n", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	m := d.mountpoint(r.Name)

	if s, ok := d.volumes[m]; ok {
		if s.connections <= 1 {
			// TODO Actually delete the RBD
			delete(d.volumes, m)
		}
	}
	return dkvolume.Response{}
}

func (d cephDriver) Path(r dkvolume.Request) dkvolume.Response {
	return dkvolume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d cephDriver) Mount(r dkvolume.Request) dkvolume.Response {
	log.Printf("Mount(%s)\n", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	mountpoint := d.mountpoint(r.Name)
	log.Printf("mountpoint(%s) => %s\n", r.Name, mountpoint)

	s, ok := d.volumes[mountpoint]
	if ok && s.connections > 0 {
		s.connections++
		return dkvolume.Response{Mountpoint: mountpoint}
	}

	fi, err := os.Lstat(mountpoint)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return dkvolume.Response{Err: err.Error()}
		}
	} else if err != nil {
		return dkvolume.Response{Err: err.Error()}
	}

	if fi != nil && !fi.IsDir() {
		return dkvolume.Response{Err: fmt.Sprintf("%v already exist and it's not a directory", mountpoint)}
	}

	device, err := d.mountVolume(r.Name, mountpoint);
	if err != nil {
		return dkvolume.Response{Err: err.Error()}
	}

	d.volumes[mountpoint] = &volume{name: r.Name, device: *device, connections: 1}

	return dkvolume.Response{Mountpoint: mountpoint}
}

func (d cephDriver) Unmount(r dkvolume.Request) dkvolume.Response {
	log.Printf("Unmount(%s)\n", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	mountpoint := d.mountpoint(r.Name)
	log.Printf("mountpoint(%s) => %s\n", r.Name, mountpoint)

	if volume, ok := d.volumes[mountpoint]; ok {
		if volume.connections == 1 {
			if err := d.unmountVolume(mountpoint, volume.device); err != nil {
				return dkvolume.Response{Err: err.Error()}
			}
		}
		volume.connections--
	} else {
		return dkvolume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", mountpoint)}
	}

	return dkvolume.Response{}
}

func (d *cephDriver) mountpoint(name string) string {
	return filepath.Join(d.root, name)
}

func sh(cmd string) ([]byte, error) {
	log.Println(cmd)
	return exec.Command("sh", "-c", cmd).CombinedOutput()
}

func (d *cephDriver) mountVolume(name, destination string) (*string, error) {
	cmd := fmt.Sprintf("rbd map %s", name)
	out, err := sh(cmd)
	if err != nil {
		log.Println(string(out))
		return nil, err
	}
	log.Println(string(out))
	device := string(out)
	cmd = fmt.Sprintf("mount /dev/rbd1 %s", destination)
	if out, err := sh(cmd); err != nil {
		log.Println(string(out))
		return nil, err
	}
	return &device, nil
}

func (d *cephDriver) unmountVolume(target, device string) error {
	cmd := fmt.Sprintf("umount %s", target)
	if out, err := sh(cmd); err != nil {
		log.Println(string(out))
		return err
	}
	cmd = fmt.Sprintf("rbd unmap %s", device)
	if out, err := sh(cmd); err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}
