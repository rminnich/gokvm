package virtio

import (
	"fmt"
	"net"
	"os"
	"unsafe"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/kvm"
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
	u64                          uint64
	vhostGetFeatures             = kvm.IIOR(vhostVirtio, 0x00, unsafe.Sizeof(u64))
	vhostSetFeatures             = kvm.IIOW(vhostVirtio, 0x00, unsafe.Sizeof(u64))
	vhostSetxOwner               = kvm.IIO(vhostVirtio, 0x01)
	vhostxRESETxOwner            = kvm.IIO(vhostVirtio, 0x02)
	vhostSetxMemxTable           = kvm.IIOW(vhostVirtio, 0x03, unsafe.Sizeof(vhostxmemory{}))
	vhostSetxLogBase             = kvm.IIOW(vhostVirtio, 0x04, unsigned.Sizeof(uint64))
	vhostSetxLogxFD              = kvm.IIOW(vhostVirtio, 0x07, unsafe.Sizeof(int))
	vhostSetxVringxNum           = kvm.IIOW(vhostVirtio, 0x10, unsafe.Sizeof(vhostxvringxstate{}))
	vhostSetxVringxAddr          = kvm.IIOW(vhostVirtio, 0x11, unsafe.Sizeof(vhostxvringxaddr{}))
	vhostSetxVringxBase          = kvm.IIOW(vhostVirtio, 0x12, unsafe.Sizeof(vhostxvringxstate{}))
	vhostGetxVringxBase          = kvm.IIOWR(vhostVirtio, 0x12, unsafe.Sizeof(vhostxvringxstate{}))
	vhostSetxVringxKick          = kvm.IIOW(vhostVirtio, 0x20, unsafe.Sizeof(vhostxvringxfile{}))
	vhostSetxVringxCall          = kvm.IIOW(vhostVirtio, 0x21, unsafe.Sizeof(vhostxvringxfile{}))
	vhostSetxVringxErr           = kvm.IIOW(vhostVirtio, 0x22, unsafe.Sizeof(vhostxvringxfile{}))
	vhostxNETSetxBackend         = kvm.IIOW(vhostVirtio, 0x30, unsafe.Sizeof(vhostxvringxfile{}))
	vhostxSCSISetxEndpoint       = kvm.IIOW(vhostVirtio, 0x40, unsafe.Sizeof(vhostxscsixtarget{}))
	vhostxSCSIxClearxEndpoint    = kvm.IIOW(vhostVirtio, 0x41, unsafe.Sizeof(vhostxscsixtarget{}))
	i                            int
	vhostxSCSIGetxABIxVersion    = kvm.IIOW(vhostVirtio, 0x42, unsafe.Sizeof(i))
	u                            uint32
	vhostxSCSISetxEventsxxMissed = kvm.IIOW(vhostVirtio, 0x43, unsafe.Sizeof(u))
	vhostxSCSIGetxEventsxMissed  = kvm.IIOW(vhostVirtio, 0x44, unsafe.Sizeof(u))
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

func NewVSock(dev string, route *flag.VSockRoute) (*VSock, error) {
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

	vr := make([]byte, 128*1024)

	fd := vs.Fd()

	return nil, errx
}
