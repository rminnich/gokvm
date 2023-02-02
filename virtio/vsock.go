package virtio

import (
	"encoding/binary"
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

type vhostVringAddr struct {
	index         uint
	flags         uint
	descUserAddr  uint64
	usedUserAddr  uint64
	availUserAddr uint64
	logGuestAddr  uint64
}

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
	f  *os.File
	fd uintptr
	// 0 is kick, 1 is call
	efd      [2]uintptr
	Local    net.Addr
	vra      *vhostVringAddr
	vm       *VhostMemory
	features uint64
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

// mem allocates memory using mmap and returns the address as uintptr and the slice.
func mem(amt uintptr) (uintptr, []byte, error) {
	prot := syscall.PROT_READ | syscall.PROT_WRITE
	flags := syscall.MAP_SHARED | syscall.MAP_ANONYMOUS | syscall.MAP_POPULATE

	m, err := syscall.Mmap(-1, 0, int(amt), prot, flags)
	if err != nil {
		return 0, nil, fmt.Errorf("mem(%#x):%w", amt, err)
	}

	return uintptr(unsafe.Pointer(&m[0])), m, err
}

// VsockMemory sets vsock memory. Not needed?
// The minimum of (nregions, kernel nregions) is used.
// There is a very strange "why does it work" thing going on here.
// In the tests in Linux, the set the Guest PA to be User PA.
// How can that work?
// It will most likely work because the guest base memory
// is mmap'ed, and so further mmaps won't overlap.
// but isn't the memory for the guest set to zero?
// What if the mmap for the rings is in the range 0..sizeof(guest memory)
// well, you'll probably still be lucky enough to have an mmap for
// the descriptors that is larger than the top of guest memory. Let's hope.
// For now, we're just going to alloc 16M of memory in one region.
func (v *VSock) Memory(nregions uint32) error {
	const mmapSize = 16 * 256 * vhostPageSize

	var (
		// Why 8? It's a variable size C struct,
		// with potentially 0 items at the end, only
		// 8 bytes of data are required.
		vhostSetMemTable = IIOW(0x03, 8)

		u64             uint64
		vhostSetLogBase = IIOW(0x04, unsafe.Sizeof(u64))
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

	vm := &VhostMemory{Nregions: nregions}
	for i := range vm.Regions[:nregions] {
		m, _, err := mem(mmapSize)
		if err != nil {
			return fmt.Errorf("region %d:%w", i, err)
		}

		vm.Regions[i].MemorySize = uint64(mmapSize)
		vm.Regions[i].UserspaceAddr = uint64(m)
		vm.Regions[i].GuestPhysAddr = uint64(m)
	}

	log.Printf("set memtable to %d regions %#x", vm.Nregions, vm.Regions[:nregions])

	if err := vhostSetMemTable.ioctl(v.fd, uintptr(unsafe.Pointer(vm))); err != nil {
		return fmt.Errorf("set memtable to %#x:%w", unsafe.Pointer(&v), err)
	}

	// Not sure if we need this, but ...
	// And how big should it be? Great questio .
	m, _, err := mem(64 * 1024)
	if err != nil {
		return fmt.Errorf("logbase:%w", err)
	}

	// no idea what this should be even.
	if err := vhostSetLogBase.ioctl(v.fd, m); err != nil {
		return fmt.Errorf("set logbase to %#x:%w", unsafe.Pointer(&m), err)
	}

	v.vm = vm

	return nil
}

// GuestCID sets the guest CID. If the passed-in CID is -1, a random CID is
// used. uint64 for the cid is an ABI requirement, though the kernel will
// error if any of the high 32 bits are used (for now).
func (v *VSock) GuestCID(cid uint64) error {
	return IIOW(0x60, unsafe.Sizeof(cid)).ioctl(v.fd, uintptr(unsafe.Pointer(&cid)))
}

// SetOwner sets the owner. This MUST be the first step, for now. kvm can always change.
func (v *VSock) SetOwner(owner uintptr) error {
	return IIO(1).ioctl(v.fd, owner)
}

// Features returns the features, after first setting any desired features.
func (v *VSock) Features(features ...uint64) error {
	for f := range features {
		if err := IIOW(0x00, unsafe.Sizeof(f)).ioctl(v.fd, uintptr(unsafe.Pointer(&f))); err != nil {
			return fmt.Errorf("SetFeatures(%#x): %w", f, err)
		}
	}

	if err := IIOR(0x00, unsafe.Sizeof(v.features)).ioctl(v.fd, uintptr(unsafe.Pointer(&v.features))); err != nil {
		return fmt.Errorf("GetFeatures: %w", err)
	}

	return nil
}

// Call sets up VringCall fds.
func (v *VSock) Call() error {
	for i := range v.efd[:] {
		r1, r2, err := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0x80000|0x800, 0)
		log.Printf("eventfd Syscall %d: %v, %v, %v", i, r1, r2, err)

		var errno syscall.Errno
		if ok := errors.As(err, &errno); ok && errno != 0 {
			return fmt.Errorf("eventfd %d:%w", i, err)
		}

		if err := IIOW(0x21, 0x08).ioctl(v.fd,
			uintptr(unsafe.Pointer(&vhostVringFile{index: 0, fd: int32(r1)}))); err != nil {
			return fmt.Errorf("eventfd %d vhostSetVringCall:%w", i, err)
		}

		v.efd[i] = r1
	}

	return nil
}

// VRing sets up vrings.
// It is not clear how big the rings should be, but packets are generally
// 1500 bytes or less, so allocate blocks of 2048 to keep things
// reasonable. Further, each descriptor block will be 512 entries.
func (v *VSock) VRing() error {
	pages := uintptr(1) // descUserAddr
	pages += 1          // usedUserAddr
	pages += 1          // availUserAddr
	pages += 1          // logGuestAddr
	pages += 128        // enough for 256 packets

	mp, base, err := mem(pages * vhostPageSize)
	if err != nil {
		return fmt.Errorf("VRing:%w", err)
	}

	m := uint64(mp)
	vra := &vhostVringAddr{
		index:         0,
		flags:         0,
		descUserAddr:  m,
		usedUserAddr:  m + 1*vhostPageSize,
		availUserAddr: m + 2*vhostPageSize,
		logGuestAddr:  m + 3*vhostPageSize,
	}

	// Fill in the actual addresses
	for i := 0; i < 256; i++ {
		binary.LittleEndian.PutUint64(base[i*8:], m+uint64(4*vhostPageSize+2048*i))
	}

	v.vra = vra

	return nil
}

// SetRunning Sets the running state to a value.
// For now, the state is 0 (stopped) or 1 or more (running).
func (v *VSock) SetRunning(state uintptr) error {
	// so, state is kind of 64 bits, but the ioctl seems to require
	// a u32 or int?
	// This works just fine on little endian machines and, let's face it, that's what
	// everything is.
	return IIOW(0x61, unsafe.Sizeof(uint32(1))).ioctl(v.fd, uintptr(unsafe.Pointer(&state)))
}

// NewVsock returns a NewVsock using the supplied device name and a set of routes.
func NewVSock(dev string, cid uint64, routes flag.VSockRoutes) (*VSock, error) {
	// These steps are roughly equivalent to the vdev_info_init function in virtio_test.c
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	v := &VSock{f: f, fd: f.Fd()}

	if err := v.SetOwner(0); err != nil {
		return nil, err
	}

	if err := v.Features(); err != nil {
		return nil, err
	}

	log.Printf("features %#x", v.features)

	if err := v.Memory(1); err != nil {
		return nil, fmt.Errorf("memory(1):%w", err)
	}

	// Now we move on to the vq_info_add steps

	if err := v.Call(); err != nil {
		return nil, err
	}

	if err := v.VRing(); err != nil {
		return nil, fmt.Errorf("vring:%w", err)
	}

	if err := v.GuestCID(cid); err != nil {
		return nil, fmt.Errorf("set CID to %#x:%w", cid, err)
	}

	if err := v.SetRunning(1); err != nil {
		return nil, fmt.Errorf("set running to 1:%w", err)
	}

	return &VSock{}, nil
}
