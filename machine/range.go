package machine

import (
	"errors"
	"fmt"
	"os"

	"strings"
	"syscall"

	"github.com/bobuhiro11/gokvm/flag"
)

// memrange is a memory range.
type MemRange struct {
	Name string
	Base uint64
	Size uint64
	Data []byte
}

// ErrBadRange indicates a bad range specification.
var ErrBadRange = errors.New("bad memory range specification")

// parseMemRange parses memory range strings.
// memory is specified as base@size@filename
// If base is empty, then "current end of memory+1" is assumed.
// if size is omitted, then "size of file" is assumed.
// if filename is omitted, then anonymous mmap is used in MapRange.
func ParseMemRange(r string) (*MemRange, error) {
	f := strings.SplitN(r, "@", 3)
	if len(f) != 3 {
		return nil, fmt.Errorf("%q does not have 3 fields:%w", r, ErrSyntax)
	}
	b, err := flag.ParseSize(f[0], "")
	if err != nil {
		return nil, err
	}
	s, err := flag.ParseSize(f[1], "")
	if err != nil {
		return nil, err
	}

	return &MemRange{
		Name: f[2],
		Base: uint64(b),
		Size: uint64(s),
	}, nil
}

// MapRange maps a range into a machine.
func (r *MemRange) MapRange() error {
	// This is a simple efficiency hack, and seems to do no harm.
	if len(r.Name) == 0 {
		p, err := syscall.Mmap(-1, 0, int(r.Size),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
		if err != nil {
			return err
		}
		r.Data = p
		return nil

	}

	f, err := os.OpenFile(r.Name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Another coding anti-pattern reguired by golangci-lint.
	// Would not pass review in Google.
	if r.Data, err = syscall.Mmap(int(f.Fd()), 0, int(r.Size),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED); err != nil {
		return fmt.Errorf("map %q for %d bytes: %w", r.Name, r.Size, err)
	}

	return nil
}
