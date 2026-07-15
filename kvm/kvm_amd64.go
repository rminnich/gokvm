package kvm

import "unsafe"

// x86-only ioctl numbers. See kvm.go for the architecture-generic ones.
const (
	kvmGetMSRIndexList   = 0x02
	kvmGetSupportedCPUID = 0x05

	kvmGetEmulatedCPUID       = 0x09
	kvmGetMSRFeatureIndexList = 0x0A

	kvmSetTSSAddr         = 0x47
	kvmSetIdentityMapAddr = 0x48

	kvmReinjectControl = 0x71
	kvmCreatePIT2      = 0x77
	kvmSetClock        = 0x7B
	kvmGetClock        = 0x7C

	kvmGetSregs  = 0x83
	kvmSetSregs  = 0x84
	kvmTranslate = 0x85
	kvmInterrupt = 0x86

	kvmGetMSRS = 0x88
	kvmSetMSRS = 0x89

	kvmGetLAPIC = 0x8e
	kvmSetLAPIC = 0x8f

	kvmSetCPUID2          = 0x90
	kvmGetCPUID2          = 0x91
	kvmTRPAccessReporting = 0x92

	kvmX86SetupMCE           = 0x9C
	kvmX86GetMCECapSupported = 0x9D

	kvmSetGuestDebug = 0x9b

	kvmGetPIT2 = 0x9F
	kvmSetPIT2 = 0xA0

	kvmGetVCPUEvents = 0x9F
	kvmSetVCPUEvents = 0xA0

	kvmGetDebugRegs = 0xA1
	kvmSetDebugRegs = 0xA2

	kvmSetTSCKHz = 0xA2
	kvmGetTSCKHz = 0xA3

	kvmGetXCRS = 0xA6
	kvmSetXCRS = 0xA7

	kvmSMI = 0xB7

	kvmGetSRegs2 = 0xCC
	kvmSetSRegs2 = 0xCD
)

const (
	numInterrupts   = 0x100
	CPUIDFeatures   = 0x40000001
	CPUIDSignature  = 0x40000000
	CPUIDFuncPerMon = 0x0A
)

func SetTSCKHz(vcpuFd uintptr, freq uint64) error {
	_, err := Ioctl(vcpuFd,
		IIO(kvmSetTSCKHz), uintptr(freq))

	return err
}

func GetTSCKHz(vcpuFd uintptr) (uint64, error) {
	ret, err := Ioctl(vcpuFd,
		IIO(kvmGetTSCKHz), 0)
	if err != nil {
		return 0, err
	}

	return uint64(ret), nil
}

type ClockFlag uint32

// Bit positions for the KVM_CLOCK_* flags (see
// include/uapi/linux/kvm.h in the Linux kernel).
const (
	realtimeBit = 2
	hostTSCBit  = 3
)

const (
	TSCStable ClockFlag = 2
	Realtime  ClockFlag = (1 << realtimeBit)
	HostTSC   ClockFlag = (1 << hostTSCBit)
)

type ClockData struct {
	Clock    uint64
	Flags    uint32
	_        uint32
	Realtime uint64
	HostTSC  uint64
	_        [4]uint32
}

// SetClock sets the current timestamp of kvmclock to the value specified in its parameter.
// In conjunction with GET_CLOCK, it is used to ensure monotonicity on scenarios such as migration.
func SetClock(vmFd uintptr, cd *ClockData) error {
	_, err := Ioctl(vmFd,
		IIOW(kvmSetClock, unsafe.Sizeof(ClockData{})),
		uintptr(unsafe.Pointer(cd)))

	return err
}

// GetClock gets the current timestamp of kvmclock as seen by the current guest.
// In conjunction with SET_CLOCK, it is used to ensure monotonicity on scenarios such as migration.
func GetClock(vmFd uintptr, cd *ClockData) error {
	_, err := Ioctl(vmFd,
		IIOR(kvmGetClock, unsafe.Sizeof(ClockData{})),
		uintptr(unsafe.Pointer(cd)))

	return err
}

// Translation is a struct for TRANSLATE queries.
type Translation struct {
	// LinearAddress is input.
	// Most people call this a "virtual address"
	// Intel has their own name.
	LinearAddress uint64

	// This is output
	PhysicalAddress uint64
	Valid           uint8
	Writeable       uint8
	Usermode        uint8
	_               [5]uint8
}

// Translate translates a virtual address according to the vcpu’s current address translation mode.
func Translate(vcpuFd uintptr, t *Translation) error {
	_, err := Ioctl(vcpuFd,
		IIOWR(kvmTranslate, unsafe.Sizeof(Translation{})),
		uintptr(unsafe.Pointer(t)))

	return err
}

type Exception struct {
	Inject       uint8
	Nr           uint8
	HadErrorCode uint8
	Pending      uint8
	ErrorCode    uint32
}

type Interrupt struct {
	Inject uint8
	Nr     uint8
	Soft   uint8
	Shadow uint8
}

type NMI struct {
	Inject  uint8
	Pending uint8
	Masked  uint8
	_       uint8
}

type SMI struct {
	SMM          uint8
	Pening       uint8
	SMMInsideNMI uint8
	LatchedInit  uint8
}

type VCPUEvents struct {
	E                   Exception
	I                   Interrupt
	N                   NMI
	SipiVector          uint32
	Flags               uint32
	S                   SMI
	TripleFault         uint8
	_                   [26]uint8
	ExceptionHasPayload uint8
	ExceptionPayload    uint64
}

// GetVCPUEvents gets currently pending exceptions, interrupts, and NMIs as well as related states of the vcpu.
func GetVCPUEvents(vcpuFd uintptr, event *VCPUEvents) error {
	_, err := Ioctl(vcpuFd,
		IIOR(kvmGetVCPUEvents, unsafe.Sizeof(VCPUEvents{})),
		uintptr(unsafe.Pointer(event)))

	return err
}

// SetVCPUEvents sets spending exceptions, interrupts, and NMIs as well as related states of the vcpu.
func SetVCPUEvents(vcpuFd uintptr, event *VCPUEvents) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmSetVCPUEvents, unsafe.Sizeof(VCPUEvents{})),
		uintptr(unsafe.Pointer(event)))

	return err
}

// SMI queues an SMI on the thread’s vcpu.
func PutSMI(vcpuFd uintptr) error {
	_, err := Ioctl(vcpuFd, IIO(kvmSMI), 0)

	return err
}
