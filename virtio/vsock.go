package virtio

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
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

// type vhostMemoryRegion struct {
// 	guestPhysAddr uint64
// 	memorySize    uint64
// 	userspaceAddr uint64
// 	_             uint64
// }

// type vhostMemory struct {
// 	nregions uint32
// 	padding  uint32
// 	regions  []vhostMemoryRegion
// }

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

func NewVSock(dev string, cid uint64, routes flag.VSockRoutes) (*VSock, error) {
	var (
		u64                   uint64
		vhostGetFeatures      = IIOR(0x00, unsafe.Sizeof(u64))
		vhostSetOwner         = IIO(0x01)
		vhostSetVringCall     = IIOW(0x21, 0x08) //  unsafe.Sizeof(vhostVringFile{})) // 0x4008af21
		vhostVsockSetGuestCID = IIOW(0x60, unsafe.Sizeof(u64))
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

	mmapSize := 16 * vhostPageSize

	_, err = syscall.Mmap(-1, 0, mmapSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		return nil, err
	}

	fd := vs.Fd()
	if err := vhostSetOwner.ioctl(fd, 0); err != nil {
		return nil, err
	}

	var features uint64
	if err := vhostGetFeatures.ioctl(fd, uintptr(unsafe.Pointer(&features))); err != nil {
		return nil, fmt.Errorf("GetFeatures: %w", err)
	}

	log.Printf("features %#x", features)

	// Must have en eventfd2
	// system call $ not defined anywhere?
	// EFD_CLOEXEC                                 = 0x80000
	// EFD_NONBLOCK                                = 0x800
	r1, r2, err := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0x80000|0x800, 0)
	log.Printf("info:eventfd1; %v, %v, %v", r1, r2, err)

	var errno syscall.Errno
	if ok := errors.As(err, &errno); ok && errno != 0 {
		return nil, fmt.Errorf("first eventfd:%w", err)
	}

	// not sure what's up here.
	// 1874054 ioctl(12, _IOC(_IOC_WRITE, 0xaf, 0x21, 0x10), 0xc000113d98) = -1 ENOTTY (Inappropriate ioctl for device)
	// so strace thinks this ioctl is wrong.
	if err := vhostSetVringCall.ioctl(fd,
		uintptr(unsafe.Pointer(&vhostVringFile{index: 0, fd: int32(r1)}))); err != nil {
		return nil, fmt.Errorf("first vhostSetVringCall:%w", err)
	}

	r1, r2, err = syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0x80000|0x800, 0)
	log.Printf("info:eventfd2; %v, %v, %v", r1, r2, err)

	if ok := errors.As(err, &errno); ok && errno != 0 {
		return nil, fmt.Errorf("second eventfd:%w", err)
	}

	// not sure what's up here.
	// 1874054 ioctl(12, _IOC(_IOC_WRITE, 0xaf, 0x21, 0x10), 0xc000113d98) = -1 ENOTTY (Inappropriate ioctl for device)
	// so strace thinks this ioctl is wrong.
	// this line length is terrible.
	if err := vhostSetVringCall.ioctl(fd,
		uintptr(unsafe.Pointer(&vhostVringFile{index: 1, fd: int32(r1)}))); err != nil {
		return nil, fmt.Errorf("second vhostSetVringCall:%w", err)
	}

	if err := vhostVsockSetGuestCID.ioctl(fd, uintptr(unsafe.Pointer(&cid))); err != nil {
		return nil, fmt.Errorf("et CID to %#x:%w", cid, err)
	}

	return &VSock{}, nil
}
