package flag

import (
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// A VSockRoute is a string
// describing a VM address
// and a local address.
type VSockRoute struct {
	VMPort    uint32
	Net, Addr string
}

type VSockRoutes []*VSockRoute

// Stringer for VSockRoute
func (v *VSockRoute) String() string {
	return fmt.Sprintf("%d=%q:%q", v.VMPort, v.Net, v.Addr)
}

func (v VSockRoutes) String() string {
	var s string
	for _, r := range v {
		if len(s) > 0 {
			s = s + ","
			s = s + r.String()
		}
	}
	return s
}

// Config defines the configuration of the
// virtual machine, as determined by flags.
type Config struct {
	Dev        string
	Kernel     string
	Initrd     string
	Params     string
	TapIfName  string
	Disk       string
	NCPUs      int
	MemSize    int
	TraceCount int
	Routes     VSockRoutes
	Vsock      string
	CID        uint32
}

// ParseSize parses a size string as number[gGmMkK]. The multiplier is optional,
// and if not set, the unit passed in is used. The number can be any base and
// size.
func ParseSize(s, unit string) (int, error) {
	sz := strings.TrimRight(s, "gGmMkK")
	if len(sz) == 0 {
		return -1, fmt.Errorf("%q:can't parse as num[gGmMkK]:%w", s, strconv.ErrSyntax)
	}

	amt, err := strconv.ParseUint(sz, 0, 0)
	if err != nil {
		return -1, err
	}

	if len(s) > len(sz) {
		unit = s[len(sz):]
	}

	switch unit {
	case "G", "g":
		return int(amt) << 30, nil
	case "M", "m":
		return int(amt) << 20, nil
	case "K", "k":
		return int(amt) << 10, nil
	case "":
		return int(amt), nil
	}

	return -1, fmt.Errorf("can not parse %q as num[gGmMkK]:%w", s, strconv.ErrSyntax)
}

func ParseRoutes(route ...string) (VSockRoutes, error) {
	var routes VSockRoutes

	for _, r := range route {
		a := strings.Split(r, "=")
		if len(a) != 2 {
			return nil, fmt.Errorf("%q not in form vsockport=net:port:%w", r, strconv.ErrSyntax)
		}

		local := strings.Split(a[1], ":")
		if len(local) != 2 {
			return nil, fmt.Errorf("%q not in form vsockport=net:port:%w", r, strconv.ErrSyntax)
		}

		port, err := strconv.ParseUint(a[0], 0, 32)
		if err != nil {
			return nil, fmt.Errorf("%q:%w", a[0], err)
		}

		routes = append(routes, &VSockRoute{VMPort: uint32(port), Net: local[0], Addr: local[1]})
	}

	return routes, nil
}

// ParseArgs calls flag.Parse and a *Config or error.
func ParseArgs(args []string) (*Config, error) {
	c := &Config{}

	// There is almost no case where letting users pick the CID
	// ends well, so it will not be an option.
	rand.Seed(time.Now().UnixNano())
	c.CID = uint32(rand.Intn(0xffffffff-3) + 3)

	flag.StringVar(&c.Dev, "D", "/dev/kvm", "path of kvm device")
	flag.StringVar(&c.Kernel, "k", "./bzImage", "kernel image path")
	flag.StringVar(&c.Initrd, "i", "./initrd", "initrd path")
	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	flag.StringVar(&c.Params, "p", `console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`debug apic=debug show_lapic=all mitigations=off lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init`, "kernel command-line parameters")
	flag.StringVar(&c.TapIfName, "t", "tap", "name of tap interface")
	flag.StringVar(&c.Disk, "d", "/dev/zero", "path of disk file (for /dev/vda)")
	flag.StringVar(&c.Vsock, "vsock", "", "vsock device: default empty, set to /dev/vhost-vsock to try it")

	flag.IntVar(&c.NCPUs, "c", 1, "number of cpus")

	msize := flag.String("m", "1G", "memory size: as number[gGmM], optional units, defaults to G")
	tc := flag.String("T", "0", "how many instructions to skip between trace prints -- 0 means tracing disabled")
	routes := flag.String("R", "", "1 or more vsock routes in the form vsockport=type:port[...], e.g. 17010=unix:file or 17010=tcp:17010")

	flag.Parse()

	var err error
	if err = flag.CommandLine.Parse(args[1:]); err != nil {
		return nil, err
	}

	if c.MemSize, err = ParseSize(*msize, "g"); err != nil {
		return nil, err
	}

	if c.TraceCount, err = ParseSize(*tc, ""); err != nil {
		return nil, err
	}

	if len(*routes) == 0 {
		return c, nil
	}

	if c.Routes, err = ParseRoutes(strings.Split(*routes, ",")...); err != nil {
		return nil, err
	}

	return c, nil
}
