package virtio

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/flag"
)

const (
	// currently unused, left here for reference.
	// vhostVringflog        = 0.
	vhostPageSize = 0x1000
	// vhostFLogAll          = 26
	// vhostNetFVirtioNetHdr = 27
	// vhostSCSIABIVersion   = 1.
)

// NOTE: these were auto-generated but whatever.
// until we use them, we can leave them commented out.
// type vhostVringState struct {
// 	index uint
// 	num   uint
// }

type vhostVringFile struct {
	index int32
	fd    int32
}

// type vhostVringAddr struct {
// 	index         uint
// 	flags         uint
// 	descUserAddr  uint64
// 	usedUserAddr  uint64
// 	availUserAddr uint64
// 	logGuestAddr  uint64
// }

// VhostMemoryRegion defines a single memory region.
type VhostMemoryRegion struct {
	GuestPhysAddr uint64
	MemorySize    uint64
	UserspaceAddr uint64
	_             uint64
}

// VhostMemory defines the number of regions and the regions.
// It uses a fixed array, not a slice, to simplify
// the ioctl. We are unlikely to ever have more than 64
// regions anyway.
type VhostMemory struct {
	Nregions uint32
	_        uint32
	Regions  [64]VhostMemoryRegion
}

// These probably should be created as consts via go generate.
var (

// _i                int.
// golangci-lint is broken, does not see that _u64 is used in the
// next line.
// _vhostSetFeatures = IIOW(0x00, unsafe.Sizeof(_u64)).
// _vhostRESETOwner    = IIO(002)
// _vhostSetMemTable   = IIOW(0x03, unsafe.Sizeof(vhostMemory{}))
// _vhostSetLogBase    = IIOW(0x04, unsafe.Sizeof(_u64))
// _vhostSetLogFD = IIOW(0x07, unsafe.Sizeof(_i))
// _vhostSetVringNum   = IIOW(0x10, unsafe.Sizeof(vhostVringState{}))
// _vhostSetVringAddr  = IIOW(0x11, unsafe.Sizeof(vhostVringAddr{}))
// _vhostSetVringBase  = IIOW(0x12, unsafe.Sizeof(vhostVringState{}))
// _vhostGetVringBase  = IIOWR(0x12, unsafe.Sizeof(vhostVringState{}))
// _vhostSetVringKick  = IIOW(0x20, unsafe.Sizeof(vhostVringFile{})).
// _vhostSetVringErr   = IIOW(0x22, unsafe.Sizeof(vhostVringFile{}))
// _vhostNETSetBackend = IIOW(0x30, unsafe.Sizeof(vhostVringFile{}))
// vhostSCSISetEndpoint   = IIOW(0x40, unsafe.Sizeof(vhostSCSITarget{}))
// vhostSCSIClearEndpoint = IIOW(0x41, unsafe.Sizeof(vhostSCSITarget{})).

// _vhostSCSIGetABIVersion    = IIOW(0x42, unsafe.Sizeof(_i))
// _u uint32
// _vhostSCSISetEventsxMissed = IIOW(0x43, unsafe.Sizeof(_u))
// _vhostSCSIGetEventsMissed  = IIOW(0x44, unsafe.Sizeof(_u))

//  int in c is 32 bits, not sure what it is in go really.
// _vhostVsockSetRunning = IIOW(0x61, unsafe.Sizeof(_u))
)

// VSock is a single instance of a vsock connection.
type VSock struct {
	fd    [2]uintptr
	Local net.Addr
}

var errx = fmt.Errorf("not yet")

func (v *VSock) Rx() error {
	return errx
}

func (v *VSock) Tx() error {
	return errx
}

const (
	nrbits   = 8
	typebits = 8
	sizebits = 14
	dirbits  = 2

	nrmask   = (1 << nrbits) - 1
	sizemask = (1 << sizebits) - 1
	dirmask  = (1 << dirbits) - 1

	none      = 0
	write     = 1
	read      = 2
	readwrite = 3

	nrshift   = 0
	typeshift = nrshift + nrbits
	sizeshift = typeshift + typebits
	dirshift  = sizeshift + sizebits
)

// KVMIO is for the KVMIO ioctl.
const KVMVSOCK = 0xAF

type ioval uintptr

// IIOWR creates an IIOWR ioctl.
func IIOWR(nr, size uintptr) ioval {
	return IIOC(readwrite, nr, size)
}

// IIOR creates an IIOR ioctl.
func IIOR(nr, size uintptr) ioval {
	return IIOC(read, nr, size)
}

// IIOW creates an IIOW ioctl.
func IIOW(nr, size uintptr) ioval {
	return IIOC(write, nr, size)
}

// IIO creates an IIOC ioctl from a number.
func IIO(nr uintptr) ioval {
	return IIOC(none, nr, 0)
}

// IIOC creates an IIOC ioctl from a direction, nr, and size.
func IIOC(dir, nr, size uintptr) ioval {
	// This is another case of forced wrapping which is considered an anti-pattern in Google.
	return ioval(((dir & dirmask) << dirshift) | (KVMVSOCK << typeshift) |
		((nr & nrmask) << nrshift) | ((size & sizemask) << sizeshift))
}

func (i ioval) ioctl(fd, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(i), arg)
	if errno != 0 {
		return errno
	}

	return nil
}

// VsockMemory sets vsock memory. Not needed?
// The minimum of (nregions, kernel nregions) is used.
func VsockMemory(fd uintptr, nregions uint32) (*VhostMemory, error) {
	const mmapSize = 16 * vhostPageSize

	var (
		vhostSetMemTable = IIOW(0x03, 8) // variable size C struct,
		u64              uint64
		vhostSetLogBase  = IIOW(0x04, unsafe.Sizeof(u64))
	)

	r, err := os.ReadFile("/sys/module/vhost/parameters/max_mem_regions")
	if err == nil {
		f := strings.Fields(string(r))
		if len(f) > 0 {
			x, err := strconv.ParseUint(f[0], 0, 1)
			if err == nil && x < uint64(nregions) {
				nregions = uint32(x)
			}
		}
	}

	v := &VhostMemory{Nregions: nregions}
	for i := range v.Regions[:nregions] {
		m, err := syscall.Mmap(-1, 0, mmapSize,
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_ANONYMOUS|syscall.MAP_POPULATE)
		if err != nil {
			return nil, fmt.Errorf("mmap %d bytes for region %d:%w", mmapSize, i, err)
		}

		v.Regions[i].MemorySize = uint64(mmapSize)
		v.Regions[i].UserspaceAddr = uint64(uintptr(unsafe.Pointer(&m[0])))
		// Start it at 512GiB
		v.Regions[i].GuestPhysAddr = uint64(mmapSize*i) + (512 << 20)
	}

	log.Printf("set memtable to %#x", v.Regions[:nregions])

	if err := vhostSetMemTable.ioctl(fd, uintptr(unsafe.Pointer(v))); err != nil {
		return nil, fmt.Errorf("set memtable to %#x:%w", unsafe.Pointer(&v), err)
	}

	if err := vhostSetLogBase.ioctl(fd, uintptr(v.Regions[0].UserspaceAddr)); err != nil {
		return nil, fmt.Errorf("set memtable to %#x:%w", unsafe.Pointer(&v), err)
	}

	return v, nil
}

// GuestCID sets the guest CID. If the passed-in CID is -1, a random CID is
// used. uint64 for the cid is an ABI requirement, though the kernel will
// error if any of the high 32 bits are used (for now).
func GuestCID(fd uintptr, cid uint64) error {
	return IIOW(0x60, unsafe.Sizeof(cid)).ioctl(fd, uintptr(unsafe.Pointer(&cid)))
}

// SetOwner sets the owner. This MUST be the first step, for now. kvm can always change.
func SetOwner(fd, owner uintptr) error {
	return IIO(1).ioctl(fd, owner)
}

// Features returns the features, after first setting any desired features.
func Features(fd uintptr, features ...uint64) (uint64, error) {
	for f := range features {
		if err := IIOW(0x00, unsafe.Sizeof(f)).ioctl(fd, uintptr(unsafe.Pointer(&f))); err != nil {
			return 0, fmt.Errorf("SetFeatures(%#x): %w", f, err)
		}
	}

	var f uint64
	if err := IIOR(0x00, unsafe.Sizeof(f)).ioctl(fd, uintptr(unsafe.Pointer(&f))); err != nil {
		return 0, fmt.Errorf("GetFeatures: %w", err)
	}

	return f, nil
}

// Call sets up VringCall fds.
func Call(fd uintptr) (uintptr, error) {
	r1, r2, err := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0x80000|0x800, 0)
	log.Printf("Call Syscall: %v, %v, %v", r1, r2, err)

	var errno syscall.Errno
	if ok := errors.As(err, &errno); ok && errno != 0 {
		return 0, fmt.Errorf("eventfd:%w", err)
	}

	if err := IIOW(0x21, 0x08).ioctl(fd,
		uintptr(unsafe.Pointer(&vhostVringFile{index: 0, fd: int32(r1)}))); err != nil {
		return 0, fmt.Errorf("first vhostSetVringCall:%w", err)
	}

	return r1, nil
}

// NewVsock returns a NewVsock using the supplied device name and a set of routes.
func NewVSock(dev string, cid uint64, routes flag.VSockRoutes) (*VSock, error) {
	var (
		u64 uint64
		u32 uint32 // a.k.a. int

		vhostVsockSetRunning = IIOW(0x61, unsafe.Sizeof(u32))
		vsock                = &VSock{}
	)

	// 	36865 openat(AT_FDCWD, "/dev/vhost-vsock", O_RDWR) = 37
	// 	36865 mmap(NULL, 135168, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0) = 0x7f2d4419d000
	// 	36865 ioctl(37, VHOST_SET_OWNER, 0)     = 0
	// 	36865 ioctl(37, VHOST_GET_FEATURES, 0x7ffc65fdc2e0) = 0
	// 	36865 eventfd2(0, EFD_CLOEXEC|EFD_NONBLOCK) = 38
	// 	36865 ioctl(37, VHOST_SET_VRING_CALL, 0x7ffc65fdc2f0) = 0
	// 	36865 eventfd2(0, EFD_CLOEXEC|EFD_NONBLOCK) = 39
	// 	36865 ioctl(37, VHOST_SET_VRING_CALL, 0x7ffc65fdc2f0) = 0
	// 	36865 openat(AT_FDCWD, "/sys/module/vhost/parameters/max_mem_regions", O_RDONLY) = 40
	// 	36865 newfstatat(40, "", {st_mode=S_IFREG|0444, st_size=4096, ...}, AT_EMPTY_PATH) = 0
	// 	36865 read(40, "64\n", 4096)            = 3
	// 	36865 read(40, "", 4093)                = 0
	// 	36865 close(40)                         = 0
	// 	36865 futex(0x7f2d481d1fe8, FUTEX_WAKE_PRIVATE, 2147483647) = 0
	// 36865 ioctl(37, VHOST_VSOCK_SET_GUEST_CID, 0x7ffc65fdc328) = 0
	vs, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	fd := vs.Fd()

	if err := SetOwner(fd, 0); err != nil {
		return nil, err
	}

	features, err := Features(fd)
	if err != nil {
		return nil, err
	}

	log.Printf("features %#x", features)

	for i := range vsock.fd[:] {
		vsock.fd[i], err = Call(fd)
		if err != nil {
			return nil, fmt.Errorf("evenfd %d::%w", i, err)
		}
	}

	if err := GuestCID(fd, cid); err != nil {
		return nil, fmt.Errorf("set CID to %#x:%w", cid, err)
	}

	u64 = 1
	if err := vhostVsockSetRunning.ioctl(fd, uintptr(unsafe.Pointer(&u64))); err != nil {
		return nil, fmt.Errorf("set running to 1:%w", err)
	}

	return &VSock{}, nil
}
