package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/calavera/dkvolume"

	"github.com/noahdesu/go-ceph/rados"
	"github.com/noahdesu/go-ceph/rbd"
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

func checkFs(device string) (string, error) {
	log.Printf("blkid %s", device)
 	out, err := exec.Command("blkid", device).Output()
 	if out != nil {
	 	sout := strings.TrimRight(string(out), "\n")
		return sout, err
 	}
 	return "", err
}

func ensureFs(name string) dkvolume.Response {
	log.Printf("ensureFs(%q)", name)
	device, err := rbdMap(name)
	if err != nil {
		return dkvolume.Response{Err: err.Error()}
	}
	defer rbdUnmap(*device)

	t, err := checkFs(*device)
	log.Printf("t => %q", t)
	if !strings.HasSuffix(t, "TYPE=\"xfs\"") {
		log.Printf("Formatting %s", *device)
		out, _, err := sh(fmt.Sprintf("mkfs.xfs -f %s", *device))
		if err != nil {
			return dkvolume.Response{Err: err.Error()}
		}
		log.Printf(string(out))
	}
	return dkvolume.Response{}
}

func rbdExists(name string) (bool, error) {
	conn, err := rados.NewConn()
	if err != nil {
		log.Fatal(err)
		return false, err
	}
	conn.ReadDefaultConfigFile()
	conn.Connect()
	defer conn.Shutdown()

	ioContext, err := conn.OpenIOContext("rbd")
	if err != nil {
		log.Fatal(err)
		return false, err
	}
	defer ioContext.Destroy()

	volumes, err := rbd.GetImageNames(ioContext)
	if err != nil {
		log.Fatal(err)
		return false, err
	}
	for _, volumeName := range volumes {
		if name == volumeName {
			return true, nil
		}
	}
	return false, nil
}

func (d cephDriver) Create(r dkvolume.Request) dkvolume.Response {
	log.Printf("Create(%q)", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	name := r.Name
	mountpoint := d.mountpoint(name)

	if _, ok := d.volumes[mountpoint]; ok {
		return dkvolume.Response{}
	}

	exists, err := rbdExists(name)
	if err != nil {
		return dkvolume.Response{Err: err.Error()}
	}

	if !exists {
		if out, _, err := sh(fmt.Sprintf("rbd create --size 128 %s", name)); err != nil {
			log.Print(string(out))
			return dkvolume.Response{Err: err.Error()}
		}
	}

	return ensureFs(name)
}

func (d cephDriver) Remove(r dkvolume.Request) dkvolume.Response {
	log.Printf("Remove(%q)", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	name := r.Name
	mountpoint := d.mountpoint(name)

	if volume, ok := d.volumes[mountpoint]; ok {
		if volume.connections <= 1 {
			cmd := fmt.Sprintf("rbd rm %s", name)
			if out, _, err := sh(cmd); err != nil {
				log.Print(string(out))
				return dkvolume.Response{Err: err.Error()}
			}
			delete(d.volumes, mountpoint)
		}
	}
	return dkvolume.Response{}
}

func (d cephDriver) Path(r dkvolume.Request) dkvolume.Response {
	return dkvolume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d cephDriver) Mount(r dkvolume.Request) dkvolume.Response {
	log.Printf("Mount(%q)", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	mountpoint := d.mountpoint(r.Name)
	log.Printf("mountpoint(%q) => %q", r.Name, mountpoint)

	vol, ok := d.volumes[mountpoint]
	if ok && vol.connections > 0 {
		vol.connections++
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
	log.Printf("Unmount(%q)\n", r.Name)
	d.m.Lock()
	defer d.m.Unlock()
	mountpoint := d.mountpoint(r.Name)
	log.Printf("mountpoint(%q) => %s\n", r.Name, mountpoint)

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

func execCommand(name string, arg... string) ([]byte, []byte, error) {
 	cmd := exec.Command(name, arg...)
	stdout, err := cmd.StdoutPipe(); if err != nil {
		return nil, nil, err
	}
 	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	sout, err := ioutil.ReadAll(stdout); if err != nil {
		return nil, nil, err
	}
	serr, err := ioutil.ReadAll(stderr); if err != nil {
		return nil, nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, nil, err
	}
	return sout, serr, nil
}

func sh(s string) ([]byte, []byte, error) {
	log.Print(s)
	return execCommand("sh", "-c", s)
}

func rbdMap(name string) (*string, error) {
	log.Printf("rbdMap(%q)", name)
	out, _, err := sh(fmt.Sprintf("rbd map %s", name))
	if err != nil {
		log.Print(string(out))
		return nil, err
	}
	device := strings.TrimRight(string(out), "\n")
	log.Printf("rbd map %s => %q", name, device)
	return &device, nil
}

func rbdUnmap(device string) error {
	log.Printf("rbdUnmap(%q)", device)
	if out, _, err := sh(fmt.Sprintf("rbd unmap %s", device)); err != nil {
		log.Print(string(out))
		return err
	}
	return nil
}

func (d *cephDriver) mountVolume(name, target string) (*string, error) {
	log.Printf("mountVolume(%q, %q)", name, target)
	device, err := rbdMap(name)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	log.Printf("rbdMap(%q) => %q", name, *device)
	if out, _, err := sh(fmt.Sprintf("mount %s %s", *device, target)); err != nil {
		log.Print(string(out))
		return nil, err
	}
	return device, nil
}

func (d *cephDriver) unmountVolume(target, device string) error {
	log.Printf("unmountVolume(%q, %q)", target, device)
	cmd := fmt.Sprintf("umount %s", target)
	if out, _, err := sh(cmd); err != nil {
		log.Print(string(out))
		return err
	}
	return rbdUnmap(device)
}
