package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/calavera/dkvolume"
)

const (
	cephId   = "_ceph"
	socketAddress = "/usr/share/docker/plugins/ceph.sock"
)

var (
	defaultDir  = filepath.Join(dkvolume.DefaultDockerRootDirectory, cephId)
	root        = flag.String("root", defaultDir, "GlusterFS volumes root directory")
	pool = flag.String("pool", "rbd", "Ceph pool to use for volumes")
)

func main() {
	flag.Parse()
	d := newCephDriver(*root, *pool)
	h := dkvolume.NewHandler(d)
	fmt.Printf("listening on %s\n", socketAddress)
	fmt.Println(h.ServeUnix("root", socketAddress))
}
