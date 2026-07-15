package vmm

import (
	"errors"
	"fmt"
	"os"

	"github.com/bobuhiro11/gokvm/machine"
	"golang.org/x/sync/errgroup"
)

// ErrUnsupportedOnARM64 indicates a feature that the amd64 vmm
// supports but which is not yet implemented for arm64 (e.g. PVH
// firmware boot, virtio net/disk, or an initrd/device-tree-dependent
// boot -- see machine.Machine's package doc for the arm64 scope note).
var ErrUnsupportedOnARM64 = errors.New("not yet supported on arm64")

// Init instantiates a machine.
func (v *VMM) Init() error {
	if v.TapIfName != "" {
		return fmt.Errorf("tap device: %w", ErrUnsupportedOnARM64)
	}

	if v.Disk != "" {
		return fmt.Errorf("disk: %w", ErrUnsupportedOnARM64)
	}

	m, err := machine.New(v.Dev, v.NCPUs, v.MemSize)
	if err != nil {
		return err
	}

	v.Machine = m

	return nil
}

// Setup loads a raw arm64 Image kernel. PVH firmware and initrds are
// not yet supported on arm64.
func (v *VMM) Setup() error {
	if v.Initrd != "" {
		return fmt.Errorf("initrd: %w", ErrUnsupportedOnARM64)
	}

	kern, err := os.Open(v.Kernel)
	if err != nil {
		return err
	}
	defer kern.Close()

	return v.Machine.LoadLinux(kern, v.Params)
}

// Boot runs all vcpus to completion. There is no interactive console
// (input) support yet; guest output written to the minimal MMIO UART
// is printed to stdout.
func (v *VMM) Boot() error {
	g := new(errgroup.Group)

	for cpu := 0; cpu < v.NCPUs; cpu++ {
		i := cpu

		fmt.Printf("Start CPU %d of %d\r\n", i, v.NCPUs)

		g.Go(func() error {
			return v.RunInfiniteLoop(i)
		})
	}

	fmt.Printf("Waiting for CPUs to exit\r\n")

	if err := g.Wait(); err != nil && !errors.Is(err, machine.ErrMachineStopped) {
		return err
	}

	fmt.Printf("All cpus done\n\r")

	return nil
}
