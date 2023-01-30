package virtio

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/flag"
)

const (
	vhostVringflog        = 0
	vhostPageSize         = 0x1000
	vhostVirtio           = 0xAF
	vhostFLogAll          = 26
	vhostNetFVirtioNetHdr = 27
	vhostSCSIABIVersion   = 1
)

// NOTE: these were auto-generated but whatever.
type vhostVringState struct {
	index uint
	num   uint
}
type vhostVringFile struct {
	index uint
	fd    int
}

type vhostVringAddr struct {
	index         uint
	flags         uint
	descUserAddr  uint64
	usedUserAddr  uint64
	availUserAddr uint64
	logGuestAddr  uint64
}

type vhostMemoryRegion struct {
	guestPhysAddr uint64
	memorySize    uint64
	userspaceAddr uint64
	_             uint64
}

type vhostMemory struct {
	nregions uint32
	padding  uint32
	regions  []vhostMemoryRegion
}

var (
	u64                uint64
	i                  int
	vhostGetFeatures   = IIOR(0x00, unsafe.Sizeof(u64))
	vhostSetFeatures   = IIOW(0x00, unsafe.Sizeof(u64))
	vhostSetOwner      = IIO(0x01)
	vhostRESETxOwner   = IIO(002)
	vhostSetMemTable   = IIOW(0x03, unsafe.Sizeof(vhostMemory{}))
	vhostSetLogBase    = IIOW(0x04, unsafe.Sizeof(u64))
	vhostSetLogFD      = IIOW(0x07, unsafe.Sizeof(i))
	vhostSetVringNum   = IIOW(0x10, unsafe.Sizeof(vhostVringState{}))
	vhostSetVringAddr  = IIOW(0x11, unsafe.Sizeof(vhostVringAddr{}))
	vhostSetVringBase  = IIOW(0x12, unsafe.Sizeof(vhostVringState{}))
	vhostGetVringBase  = IIOWR(0x12, unsafe.Sizeof(vhostVringState{}))
	vhostSetVringKick  = IIOW(0x20, unsafe.Sizeof(vhostVringFile{}))
	vhostSetVringCall  = IIOW(0x21, unsafe.Sizeof(vhostVringFile{}))
	vhostSetVringErr   = IIOW(0x22, unsafe.Sizeof(vhostVringFile{}))
	vhostNETSetBackend = IIOW(0x30, unsafe.Sizeof(vhostVringFile{}))
	//vhostSCSISetEndpoint   = IIOW(0x40, unsafe.Sizeof(vhostSCSITarget{}))
	//vhostSCSIClearEndpoint = IIOW(0x41, unsafe.Sizeof(vhostSCSITarget{}))

	vhostSCSIGetABIVersion    = IIOW(0x42, unsafe.Sizeof(i))
	u                         uint32
	vhostSCSISetEventsxMissed = IIOW(0x43, unsafe.Sizeof(u))
	vhostSCSIGetEventsMissed  = IIOW(0x44, unsafe.Sizeof(u))
)

// VSock is a single instance of a vsock connection
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

var cid int

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

func (i ioval) ioctl(fd, arg uintptr) (uintptr, error) {
	res, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(i), arg)
	if errno != 0 {
		return res, errno
	}

	return res, nil
}

func NewVSock(dev string, routes flag.VSockRoutes) (*VSock, error) {
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

	mmapSize := 128 * 1024
	vr, err := syscall.Mmap(-1, 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED | syscall.MAP_ANONYMOUS)
	if err != nil {
		return nil, err
	}
	if false { log.Printf("%#x", vr) }

	fd := vs.Fd()

	if _, err := vhostSetOwner.ioctl(fd, 0); err != nil {
		return nil, err
	}

	var features uint64
	if _, err := vhostGetFeatures.ioctl(fd, uintptr(unsafe.Pointer(&features)));  err != nil {
		return nil, fmt.Errorf("GetFeatures: %w", err)
	}
	log.Printf("features %#x", features)

	return nil, errx
}
