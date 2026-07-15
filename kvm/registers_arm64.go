package kvm

import "unsafe"

// Regs mirrors the arm64 struct kvm_regs from the Linux KVM UAPI
// (arch/arm64/include/uapi/asm/kvm.h). Unlike x86, the general purpose
// registers, SP/PC/PSTATE, the EL1 banked registers, and the FP/SIMD
// state are all read and written together via KVM_GET_REGS/KVM_SET_REGS.
type Regs struct {
	// Regs holds the 31 general purpose registers, X0-X30.
	Regs [31]uint64

	// SP is the stack pointer (sp_el0).
	SP uint64
	// PC is the program counter.
	PC uint64
	// Pstate is the processor state (NZCV, mode bits, etc).
	Pstate uint64

	// SpEL1 is the banked SP_EL1 register.
	SpEL1 uint64
	// ELREL1 is the banked ELR_EL1 register (exception link register).
	ELREL1 uint64

	// SPSR holds the saved program status registers for each exception
	// level (KVM_NR_SPSR == 5: EL1, ABT, UND, IRQ, FIQ).
	SPSR [5]uint64

	// VRegs holds the 32 128-bit FP/SIMD registers, V0-V31, each
	// represented as two 64-bit words (low, high).
	VRegs [32][2]uint64
	FPSR  uint32
	FPCR  uint32
	_     [2]uint32
}

// GetRegs gets the general purpose (and FP/SIMD) registers for a vcpu.
func GetRegs(vcpuFd uintptr) (*Regs, error) {
	regs := &Regs{}
	_, err := Ioctl(vcpuFd, IIOR(kvmGetRegs, unsafe.Sizeof(Regs{})), uintptr(unsafe.Pointer(regs)))

	return regs, err
}

// SetRegs sets the general purpose (and FP/SIMD) registers for a vcpu.
func SetRegs(vcpuFd uintptr, regs *Regs) error {
	_, err := Ioctl(vcpuFd, IIOW(kvmSetRegs, unsafe.Sizeof(Regs{})), uintptr(unsafe.Pointer(regs)))

	return err
}

// PstateModeEL1h is the PSTATE mode bits for EL1 with SP_EL1 (i.e. the
// mode a guest kernel normally starts in).
const PstateModeEL1h = 0x00000005

// PstateFBit and PstateIBit mask FIQ and IRQ, respectively. Guests are
// normally started with both interrupts masked (PstateInit).
const (
	PstateFBit = 0x00000040
	PstateIBit = 0x00000080
	PstateInit = PstateModeEL1h | PstateFBit | PstateIBit
)
