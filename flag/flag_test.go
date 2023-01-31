package flag_test

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bobuhiro11/gokvm/flag"
)

func TestParseRoutes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		val  string
		ret  flag.VSockRoutes
		err  error
	}{
		{name: "badvsockport", val: "1x010=unix:port", ret: nil, err: strconv.ErrSyntax},
		{name: "badlocalnet", val: "1x010=unixport", ret: nil, err: strconv.ErrSyntax},
		{name: "1partno=", val: "1x010unixport", ret: nil, err: strconv.ErrSyntax},
		{name: "1partno=", val: "1x010unixport", ret: nil, err: strconv.ErrSyntax},
		{name: "2routesbad2ndrouteno=", val: "17010=tcp:17010,18010unix:port", ret: nil, err: strconv.ErrSyntax},
		{name: "2routesbad2ndrouteno:", val: "17010=tcp:17010,18010=unixport", ret: nil, err: strconv.ErrSyntax},
		{name: "2routesbadvsockportin2nd", val: "17010=tcp:17010,1x010=unixport", ret: nil, err: strconv.ErrSyntax},

		// golangci-lint antipattern
		{
			name: "uds", val: "17010=unix:port",
			ret: flag.VSockRoutes{&flag.VSockRoute{VMPort: 17010, Net: "unix", Addr: "port"}}, err: nil,
		},
		{
			name: "tcp", val: "17010=tcp:17010",
			ret: flag.VSockRoutes{&flag.VSockRoute{VMPort: 17010, Net: "tcp", Addr: "17010"}}, err: nil,
		},
		{
			name: "2ports", val: "17010=tcp:17010,18010=unix:port",
			ret: flag.VSockRoutes{
				&flag.VSockRoute{VMPort: 17010, Net: "tcp", Addr: "17010"},
				&flag.VSockRoute{VMPort: 18010, Net: "unix", Addr: "port"},
			},
			err: nil,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, err := flag.ParseRoutes(strings.Split(tt.val, ",")...)
			if !errors.Is(err, tt.err) || !reflect.DeepEqual(r, tt.ret) {
				t.Errorf("ParseRoutes(%s): ! errors.Is(%v, %v) || ! reflect.DeepEqual(%v,%v)",
					tt.val, err, tt.err, r.String(), tt.ret.String())
			}
		})
	}
}

func TestParseArg(t *testing.T) {
	t.Parallel()

	args := []string{
		"gokvm",
		"-i",
		"initrd_path",
		"-k",
		"kernel_path",
		"-p",
		"params",
		"-t",
		"tap_if_name",
		"-c",
		"2",
		"-d",
		"disk_path",
		"-m",
		"1G",
		"-T",
		"1M",
		"-R",
		"17010=unix:port",
		"-vsock", "xyz",
	}

	c, err := flag.ParseArgs(args)
	if err != nil {
		t.Fatal(err)
	}

	if c.Dev != "/dev/kvm" {
		t.Error("invalid kvm  path")
	}

	if c.Kernel != "kernel_path" {
		t.Error("invalid kernel image path")
	}

	if c.Initrd != "initrd_path" {
		t.Error("invalid initrd path")
	}

	if c.Params != "params" {
		t.Error("invalid kernel command-line parameters")
	}

	if c.TapIfName != "tap_if_name" {
		t.Error("invalid name of tap interface")
	}

	if c.Disk != "disk_path" {
		t.Errorf("invalid path of disk file: got %v, want %v", c.Disk, "disk_path")
	}

	if c.NCPUs != 2 {
		t.Error("invalid number of vcpus")
	}

	if c.MemSize != 1<<30 {
		t.Errorf("msize: got %#x, want %#x", c.MemSize, 1<<30)
	}

	if c.TraceCount != 1<<20 {
		t.Errorf("trace: got %#x, want %#x", c.TraceCount, 1<<20)
	}

	if len(c.Routes) != 1 {
		t.Errorf("trace: Routes: len(c.Routes) is %d, not 1", len(c.Routes))
	}

	if c.Vsock != "xyz" {
		t.Errorf("vsock device: got %q, want %q", c.Vsock, "xyz")
	}
}

func TestParsesize(t *testing.T) { // nolint:paralleltest
	for _, tt := range []struct {
		name string
		unit string
		m    string
		amt  int
		err  error
	}{
		{name: "badsuffix", m: "1T", amt: -1, err: strconv.ErrSyntax},
		{name: "1G", m: "1G", amt: 1 << 30, err: nil},
		{name: "1g", m: "1g", amt: 1 << 30, err: nil},
		{name: "1M", m: "1M", amt: 1 << 20, err: nil},
		{name: "1m", m: "1m", amt: 1 << 20, err: nil},
		{name: "1K", m: "1K", amt: 1 << 10, err: nil},
		{name: "1k", m: "1k", amt: 1 << 10, err: nil},
		{name: "1 with unit k", m: "1", unit: "k", amt: 1 << 10, err: nil},
		{name: "1 with unit \"\"", m: "1", unit: "", amt: 1, err: nil},
		{name: "8192m", m: "8192m", amt: 8192 << 20, err: nil},
		{name: "bogusgarbage", m: "123411;3413234134", amt: -1, err: strconv.ErrSyntax},
		{name: "bogusgarbagemsuffix", m: "123411;3413234134m", amt: -1, err: strconv.ErrSyntax},
		{name: "bogustoobig", m: "0xfffffffffffffffffffffff", amt: -1, err: strconv.ErrRange},
	} {
		amt, err := flag.ParseSize(tt.m, tt.unit)
		if !errors.Is(err, tt.err) || amt != tt.amt {
			t.Errorf("%s:parseMemSize(%s): got (%d, %v), want (%d, %v)", tt.name, tt.m, amt, err, tt.amt, tt.err)
		}
	}
}
