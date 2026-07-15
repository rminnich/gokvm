package kvm

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	kvmGetAPIVersion   = 0x00
	kvmCreateVM        = 0x1
	kvmCheckExtension  = 0x03
	kvmGetVCPUMMapSize = 0x04

	kvmCreateVCPU          = 0x41
	kvmGetDirtyLog         = 0x42
	kvmSetNrMMUPages       = 0x44
	kvmGetNrMMUPages       = 0x45
	kvmSetUserMemoryRegion = 0x46

	kvmCreateIRQChip = 0x60
	kvmGetIRQChip    = 0x62
	kvmSetIRQChip    = 0x63
	kvmIRQLineStatus = 0x67

	kvmResgisterCoalescedMMIO   = 0x67
	kvmUnResgisterCoalescedMMIO = 0x68

	kvmSetGSIRouting = 0x6A

	kvmRun     = 0x80
	kvmGetRegs = 0x81
	kvmSetRegs = 0x82

	kvmGetMPState = 0x98
	kvmSetMPState = 0x99

	kvmCreateDev = 0xE0
)

// ExitType is a virtual machine exit type.
//
//go:generate stringer -type=ExitType
type ExitType uint

const (
	EXITUNKNOWN       ExitType = 0
	EXITEXCEPTION     ExitType = 1
	EXITIO            ExitType = 2
	EXITHYPERCALL     ExitType = 3
	EXITDEBUG         ExitType = 4
	EXITHLT           ExitType = 5
	EXITMMIO          ExitType = 6
	EXITIRQWINDOWOPEN ExitType = 7
	EXITSHUTDOWN      ExitType = 8
	EXITFAILENTRY     ExitType = 9
	EXITINTR          ExitType = 10
	EXITSETTPR        ExitType = 11
	EXITTPRACCESS     ExitType = 12
	EXITS390SIEIC     ExitType = 13
	EXITS390RESET     ExitType = 14
	EXITDCR           ExitType = 15
	EXITNMI           ExitType = 16
	EXITINTERNALERROR ExitType = 17

	EXITIOIN  = 0
	EXITIOOUT = 1
)

var (
	// ErrUnexpectedExitReason is any error that we do not understand.
	ErrUnexpectedExitReason = errors.New("unexpected kvm exit reason")

	// ErrDebug is a debug exit, caused by single step or breakpoint.
	ErrDebug = errors.New("debug exit")
)

// RunData defines the data used to run a VM.
type RunData struct {
	RequestInterruptWindow     uint8
	ImmediateExit              uint8
	_                          [6]uint8
	ExitReason                 uint32
	ReadyForInterruptInjection uint8
	IfFlag                     uint8
	_                          [2]uint8
	CR8                        uint64
	ApicBase                   uint64
	Data                       [32]uint64
}

// IO interprets IO requests from a VM, by unpacking RunData.Data[0:1].
func (r *RunData) IO() (uint64, uint64, uint64, uint64, uint64) {
	// Bitfield layout of RunData.Data[0] for an IO exit: an 8-bit
	// direction, an 8-bit size, a 16-bit port, and a 32-bit count,
	// packed from the low bit upward.
	const (
		ioDirectionShift = 0
		ioDirectionMask  = 0xFF

		ioSizeShift = 8
		ioSizeMask  = 0xFF

		ioPortShift = 16
		ioPortMask  = 0xFFFF

		ioCountShift = 32
		ioCountMask  = 0xFFFFFFFF
	)

	direction := (r.Data[0] >> ioDirectionShift) & ioDirectionMask
	size := (r.Data[0] >> ioSizeShift) & ioSizeMask
	port := (r.Data[0] >> ioPortShift) & ioPortMask
	count := (r.Data[0] >> ioCountShift) & ioCountMask
	offset := r.Data[1]

	return direction, size, port, count, offset
}

// u32 truncates v to its low 32 bits. Used only for the MMIO access
// length below, which is always a small value (at most 8 bytes).
func u32(v uint64) uint32 {
	return uint32(v) //nolint:gosec // MMIO access length is always small
}

// MMIO interprets EXITMMIO requests from a VM, by unpacking
// RunData.Data[0:2]: the guest physical address being accessed, the raw
// data bytes (as a little-endian-packed uint64; only the low length
// bytes are meaningful), the access length, and whether it was a write.
func (r *RunData) MMIO() (physAddr, data uint64, length uint32, isWrite bool) {
	// Bitfield layout of RunData.Data[2] for an MMIO exit: the access
	// length in the low 32 bits, and an is-write flag at bit 32.
	const (
		mmioIsWriteShift = 32
		mmioIsWriteMask  = 0xFF
	)

	physAddr = r.Data[0]
	data = r.Data[1]
	length = u32(r.Data[2])
	isWrite = (r.Data[2]>>mmioIsWriteShift)&mmioIsWriteMask != 0

	return physAddr, data, length, isWrite
}

// GetAPIVersion gets the qemu API version, which changes rarely if at all.
func GetAPIVersion(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmGetAPIVersion), uintptr(0))
}

// CreateVM creates a KVM from the KVM device fd, i.e. /dev/kvm.
func CreateVM(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmCreateVM), uintptr(0))
}

// CreateVCPU creates a single virtual CPU from the virtual machine FD.
// Thus, the progression:
// fd from opening /dev/kvm
// vmfd from creating a vm from the fd
// vcpu fd from the vmfd.
func CreateVCPU(vmFd uintptr, vcpuID int) (uintptr, error) {
	return Ioctl(vmFd, IIO(kvmCreateVCPU), uintptr(vcpuID))
}

// Run runs a single vcpu from the vcpufd from createvcpu.
func Run(vcpuFd uintptr) error {
	_, err := Ioctl(vcpuFd, IIO(kvmRun), uintptr(0))
	if err != nil {
		// refs: https://github.com/kvmtool/kvmtool/blob/415f92c33a227c02f6719d4594af6fad10f07abf/kvm-cpu.c#L44
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
			return nil
		}
	}

	return err
}

// GetVCPUMmapSize returns the size of the VCPU region. This size is
// required for interacting with the vcpu, as the struct size can change
// over time.
func GetVCPUMMmapSize(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmGetVCPUMMapSize), uintptr(0))
}

type DevType uint32

const (
	DevFSLMPIC20 DevType = 1 + iota
	DevFSLMPIC42
	DevXICS
	DevVFIO
	_
	DevFLIC
	_
	_
	DevXIVE
	_
	DevMAX
)

type Device struct {
	Type  uint32
	Fd    uint32
	Flags uint32
}

// CreateDev creates an emulated device in the kernel.
// The file descriptor returned in fd can be used with SET/GET/HAS_DEVICE_ATTR.
func CreateDev(vmFd uintptr, dev *Device) error {
	_, err := Ioctl(vmFd,
		IIOWR(kvmCreateDev, unsafe.Sizeof(Device{})),
		uintptr(unsafe.Pointer(dev)))

	return err
}

type MPState struct {
	State uint32
}

const (
	MPStateRunnable      uint32 = 0 + iota // x86, arm64, riscv
	MPStateUninitialized                   // x86
	MPStateInitReceived                    // x86
	MPStateHalted                          // x86
	MPStateSipiReceived                    // x86
	MPStateStopped                         // x86
	MPStateCheckStop                       // s390, arm64, riscv
	MPStateOperating                       // s390
	MPStateLoad                            // s390
	MPStateApResetHold                     // s390
	MPStateSuspended                       // arm64
)

// GetMPState returns the vcpu’s current multiprocessing state.
func GetMPState(vcpuFd uintptr, mps *MPState) error {
	_, err := Ioctl(vcpuFd,
		IIOR(kvmGetMPState, unsafe.Sizeof(MPState{})),
		uintptr(unsafe.Pointer(mps)))

	return err
}

// SetMPState sets the vcpu’s current multiprocessing state.
func SetMPState(vcpuFd uintptr, mps *MPState) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmSetMPState, unsafe.Sizeof(MPState{})),
		uintptr(unsafe.Pointer(mps)))

	return err
}
