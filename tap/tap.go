package tap

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const ifNameSize = 0x10

const (
	iffVnetHdr = 0x4000 // IFF_VNET_HDR: 10-byte vnet hdr on every read/write
)

const (
	// TUNSETOFFLOAD feature flags for tap GSO offload.
	tunFCsum   = 0x01 // checksum offload
	tunFTSO4   = 0x02 // TCP segmentation offload IPv4
	tunFTSO6   = 0x04 // TCP segmentation offload IPv6
	tunFTSOECN = 0x08 // TCP ECN offload
)

// TUNSETOFFLOAD: _IOW('T', 208, unsigned int).
// syscall pkg lacks it; x/sys/unix has it but not the TUN_F_ flags.
const tunSetOffload = 0x400454d0

type Tap struct {
	fd      int
	vnetHdr bool // IFF_VNET_HDR applied: reads/writes carry a vnet hdr
	offload bool // TUNSETOFFLOAD accepted: tun performs GSO/checksum
}

type ifReq struct {
	Name  [ifNameSize]byte
	Flags uint16
	_     [0x28 - ifNameSize - 2]byte
}

func ioctl(fd, op, arg uintptr) (uintptr, error) {
	for {
		res, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL, fd, op, arg)
		if errno == syscall.EINTR {
			continue
		}

		if errno != 0 {
			return res, errno
		}

		return res, nil
	}
}

func fcntl(fd, op, arg uintptr) (uintptr, error) {
	for {
		res, _, errno := syscall.Syscall(
			syscall.SYS_FCNTL, fd, op, arg)
		if errno == syscall.EINTR {
			continue
		}

		if errno != 0 {
			return res, errno
		}

		return res, nil
	}
}

func New(name string) (*Tap, error) {
	var err error

	t := &Tap{}

	if t.fd, err = syscall.Open("/dev/net/tun", syscall.O_RDWR, 0); err != nil {
		return t, fmt.Errorf("/dev/net/tun: %w", err)
	}

	ifr := ifReq{
		Name:  [ifNameSize]byte{},
		Flags: syscall.IFF_TAP | syscall.IFF_NO_PI | iffVnetHdr,
	}
	copy(ifr.Name[:ifNameSize-1], name)

	ifrPtr := uintptr(unsafe.Pointer(&ifr))
	if _, err = ioctl(uintptr(t.fd), syscall.TUNSETIFF, ifrPtr); err != nil {
		return t, fmt.Errorf("TUN TUNSETIFF: %w", err)
	}

	// IFF_VNET_HDR requested above is honored by the kernel on
	// attach, so every read/write now carries a 10-byte vnet hdr.
	t.vnetHdr = true

	// Enable GSO/checksum offload so the tun can segment. Best
	// effort: on kernels without vnet hdr support TUNSETIFF above
	// would already have failed, so a failure here just means no
	// offload; Offload() reports it so the virtio-net device only
	// advertises GSO features when the tun can honor them.
	if _, err = ioctl(uintptr(t.fd), tunSetOffload,
		tunFCsum|tunFTSO4|tunFTSO6|tunFTSOECN); err != nil {
		t.offload = false
	} else {
		t.offload = true
	}

	var flags uintptr

	// enable non-blocking IO for tap interface
	if flags, err = fcntl(uintptr(t.fd), syscall.F_GETFL, 0); err != nil {
		return t, fmt.Errorf("TUN GETFL: %w", err)
	}

	flags |= syscall.O_NONBLOCK
	if _, err = fcntl(uintptr(t.fd), syscall.F_SETFL, flags); err != nil {
		return t, fmt.Errorf("TUN SETFL NONBLOCK: %w", err)
	}

	return t, nil
}

// FD returns the underlying file descriptor, so callers can wait
// for incoming packets with poll/epoll instead of SIGIO.
func (t *Tap) FD() int {
	return t.fd
}

// Offload reports whether the tun accepted TUNSETOFFLOAD and
// therefore performs GSO/checksum offload.
func (t *Tap) Offload() bool {
	return t.offload
}

// VnetHdr reports whether IFF_VNET_HDR was applied, meaning every
// read/write carries a 10-byte vnet hdr.
func (t *Tap) VnetHdr() bool {
	return t.vnetHdr
}

func (t *Tap) Close() error {
	return syscall.Close(t.fd)
}

func (t Tap) Write(buf []byte) (n int, err error) {
	for {
		n, err = syscall.Write(t.fd, buf)
		if errors.Is(err, syscall.EINTR) {
			continue
		}

		return n, err
	}
}

// Writev writes the buffers in a single syscall, so TX can point
// iovecs directly at guest memory instead of copying the packet
// into a contiguous buffer first.
func (t Tap) Writev(bufs [][]byte) (n int, err error) {
	for {
		n, err = unix.Writev(t.fd, bufs)
		if errors.Is(err, syscall.EINTR) {
			continue
		}

		return n, err
	}
}

func (t Tap) Read(buf []byte) (n int, err error) {
	for {
		n, err = syscall.Read(t.fd, buf)
		if errors.Is(err, syscall.EINTR) {
			continue
		}

		return n, err
	}
}
