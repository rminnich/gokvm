package kvm

import "unsafe"

// pitConfig defines properties of a programmable interrupt timer.
type pitConfig struct {
	Flags uint32
	_     [15]uint32
}

// CreatePIT2 creates a PIT type 2. Just having one was not enough.
func CreatePIT2(vmFd uintptr) error {
	pit := pitConfig{
		Flags: 0,
	}
	_, err := Ioctl(vmFd,
		IIOW(kvmCreatePIT2, unsafe.Sizeof(pitConfig{})),
		uintptr(unsafe.Pointer(&pit)))

	return err
}

type PITChannelState struct {
	Count         uint32
	LatchedCount  uint16
	CountLatched  uint8
	StatusLatched uint8
	Status        uint8
	ReadState     uint8
	WriteState    uint8
	WriteLatch    uint8
	RWMode        uint8
	Mode          uint8
	BCD           uint8
	Gate          uint8
	CountLoadTime int64
}

type PITState2 struct {
	Channels [3]PITChannelState
	Flags    uint32
	_        [9]uint32
}

// GetPIT2 retrieves the state of the in-kernel PIT model. Only valid after KVM_CREATE_PIT2.
func GetPIT2(vmFd uintptr, pstate *PITState2) error {
	_, err := Ioctl(vmFd,
		IIOR(kvmGetPIT2, unsafe.Sizeof(PITState2{})),
		uintptr(unsafe.Pointer(pstate)))

	return err
}

// SetPIT2 sets the state of the in-kernel PIT model. Only valid after KVM_CREATE_PIT2.
func SetPIT2(vmFd uintptr, pstate *PITState2) error {
	_, err := Ioctl(vmFd,
		IIOW(kvmSetPIT2, unsafe.Sizeof(PITState2{})),
		uintptr(unsafe.Pointer(pstate)))

	return err
}

type PICState struct {
	LastIRR                uint8 /* edge detection */
	IRR                    uint8 /* interrupt request register */
	IMR                    uint8 /* interrupt mask register */
	ISR                    uint8 /* interrupt service register */
	PriorityAdd            uint8 /* highest irq priority */
	IRQBase                uint8
	ReadRegSelect          uint8
	Poll                   uint8
	SpecialMask            uint8
	InitState              uint8
	AutoEOI                uint8
	RotateOnAutoEOI        uint8
	SpecialFullyNestedMode uint8
	Init4                  uint8 /* true if 4 byte init */
	ELCR                   uint8 /* PIIX edge/trigger selection */
	ELCRMask               uint8
}

// InjectInterrupt queues a hardware interrupt vector to be injected.
func InjectInterrupt(vcpuFd uintptr, intr uint32) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmInterrupt, 4),
		uintptr(intr))

	return err
}

const LAPICRegSize = 0x400

type LAPICState struct {
	Regs [LAPICRegSize]byte
}

// GetLocalAPIC reads the Local APIC registers and copies them into the input argument.
func GetLocalAPIC(vcpuFd uintptr, lapic *LAPICState) error {
	_, err := Ioctl(vcpuFd,
		IIOR(kvmGetLAPIC, unsafe.Sizeof(LAPICState{})),
		uintptr(unsafe.Pointer(lapic)))

	return err
}

// SetLocalAPIC copies the input argument into the Local APIC registers.
func SetLocalAPIC(vcpuFd uintptr, lapic *LAPICState) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmSetLAPIC, unsafe.Sizeof(LAPICState{})),
		uintptr(unsafe.Pointer(lapic)))

	return err
}

// ReinjectControl sets i8254 Inject mode.
func ReinjectControl(vmFd uintptr, mode uint8) error {
	tmp := struct {
		pitReinject uint8
		_           [31]byte
	}{
		pitReinject: mode,
	}
	_, err := Ioctl(vmFd,
		IIO(kvmReinjectControl), uintptr(unsafe.Pointer(&tmp)))

	return err
}
