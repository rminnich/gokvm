package flag

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

var ErrorInvalidSubcommands = errors.New("expected 'boot' or 'probe' subcommands")

type BootArgs struct {
	Kernel     string
	MemSize    int
	NCPUs      int
	Dev        string
	Initrd     string
	Params     string
	TapIfName  string
	Disk       string
	TraceCount int
	Resume     string
	SavePath   string
}

func parseBootArgs(args []string) (*BootArgs, error) {
	bootCmd := flag.NewFlagSet("boot subcommand", flag.ExitOnError)
	c := &BootArgs{}

	bootCmd.StringVar(&c.Dev, "D", "/dev/kvm", "path of kvm device")
	bootCmd.StringVar(&c.Kernel, "k", "./bzImage", "kernel image path")
	bootCmd.StringVar(&c.Initrd, "i", "", "initrd path")
	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	bootCmd.StringVar(&c.Params, "p", `console=ttyS0 earlyprintk=serial `+
		`noapic noacpi pci=conf1 reboot=k panic=1 `+
		`i8042.direct=1 i8042.dumbkbd=1 i8042.nopnp=1 i8042.noaux=1 `+
		`mitigations=off `+
		`pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init `+
		`kunit.enable=0`,
		"kernel command-line parameters")
	bootCmd.StringVar(&c.TapIfName, "t", "", `name of tap interface. `+
		`If the string is an empty, no tap intarface is created. (default"")`)
	bootCmd.StringVar(&c.Disk, "d", "", "path of disk file (for /dev/vda)")

	bootCmd.IntVar(&c.NCPUs, "c", 1, "number of cpus")

	msize := bootCmd.String("m", "1G",
		"memory size: as number[gGmM], optional units, defaults to G")
	tc := bootCmd.String("T", "0",
		"how many instructions to skip between trace prints -- 0 means tracing disabled")

	bootCmd.StringVar(&c.Resume, "R", "", "resume from state file")
	bootCmd.StringVar(&c.SavePath, "S", "gokvm.state", "path to write state on save")

	var err error

	if err = bootCmd.Parse(args); err != nil {
		return nil, err
	}

	if c.MemSize, err = ParseSize(*msize, "g"); err != nil {
		return nil, err
	}

	if c.TraceCount, err = ParseSize(*tc, ""); err != nil {
		return nil, err
	}

	return c, nil
}

type ProbeArgs struct{}

func parseProbeArgs(args []string) (*ProbeArgs, error) {
	probeCmd := flag.NewFlagSet("probe subcommand", flag.ExitOnError)
	c := &ProbeArgs{}

	if err := probeCmd.Parse(args); err != nil {
		return nil, err
	}

	return c, nil
}

func ParseArgs(args []string) (*BootArgs, *ProbeArgs, error) {
	if len(args) < 2 {
		return nil, nil, ErrorInvalidSubcommands
	}

	switch args[1] {
	case "boot":
		conf, err := parseBootArgs(args[2:])

		return conf, nil, err

	case "probe":
		conf, err := parseProbeArgs(args[2:])

		return nil, conf, err
	}

	return nil, nil, ErrorInvalidSubcommands
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
