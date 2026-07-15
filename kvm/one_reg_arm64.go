package kvm

import "unsafe"

// ARM64 ONE_REG ioctl numbers and register-ID encoding, from the Linux
// KVM UAPI (arch/arm64/include/uapi/asm/kvm.h). ONE_REG lets a caller
// get/set a single register (core register, or a system register such
// as an EL1 control register) that isn't covered by GetRegs/SetRegs.
//
// The ioctl numbers and register-ID encoding bits below are
// cross-checked against github.com/linuxboot/voodoo (MIT licensed Go
// code; see trace/kvm/ioctl_arm64.go's getOneReg/setOneReg/coreReg),
// which computes the same core-register ID as CoreRegID here
// (KVM_REG_ARM64 | KVM_REG_SIZE_U64 | KVM_REG_ARM_CORE | word-offset).
const (
	kvmGetOneReg = 0xab
	kvmSetOneReg = 0xac
)

// Register-ID encoding bits (KVM_REG_ARM64 | KVM_REG_SIZE_* | ...).
const (
	regARM64    = 0x6000000000000000
	regSizeU32  = 0x0020000000000000
	regSizeU64  = 0x0030000000000000
	regSizeU128 = 0x0040000000000000

	// regARMCore identifies a "core" register, i.e. one of the fields
	// of struct kvm_regs (as opposed to a system/coprocessor register).
	regARMCore = 0x0010 << 16
)

// regWordSize is the size, in bytes, of a 32-bit register word; ONE_REG
// core-register IDs encode a field's offset within Regs as a word
// offset rather than a byte offset.
const regWordSize = 4

// CoreRegID computes the ONE_REG register ID for a field at the given
// byte offset within Regs, e.g. CoreRegID(unsafe.Offsetof(Regs{}.PC)).
func CoreRegID(byteOffset uintptr) uint64 {
	return regARM64 | regSizeU64 | regARMCore | uint64(byteOffset/regWordSize)
}

// Convenience register IDs for the core registers most commonly
// accessed individually (e.g. to set the initial PC before first Run).
// These are computed with the same arithmetic as CoreRegID, but as
// compile-time constants (unsafe.Offsetof on a directly-addressed
// field is itself a constant expression) so they don't need to be
// package-level vars.
const (
	RegPC     = regARM64 | regSizeU64 | regARMCore | uint64(unsafe.Offsetof(Regs{}.PC)/regWordSize)
	RegSP     = regARM64 | regSizeU64 | regARMCore | uint64(unsafe.Offsetof(Regs{}.SP)/regWordSize)
	RegPstate = regARM64 | regSizeU64 | regARMCore | uint64(unsafe.Offsetof(Regs{}.Pstate)/regWordSize)
)

// OneRegister mirrors the arm64 struct kvm_one_reg.
type OneRegister struct {
	ID   uint64
	Addr uint64
}

// GetOneReg reads the register identified by id (see CoreRegID) into
// *val.
func GetOneReg(vcpuFd uintptr, id uint64, val *uint64) error {
	reg := OneRegister{
		ID:   id,
		Addr: uint64(uintptr(unsafe.Pointer(val))),
	}
	_, err := Ioctl(vcpuFd,
		IIOW(kvmGetOneReg, unsafe.Sizeof(OneRegister{})),
		uintptr(unsafe.Pointer(&reg)))

	return err
}

// SetOneReg writes *val into the register identified by id (see
// CoreRegID).
func SetOneReg(vcpuFd uintptr, id uint64, val *uint64) error {
	reg := OneRegister{
		ID:   id,
		Addr: uint64(uintptr(unsafe.Pointer(val))),
	}
	_, err := Ioctl(vcpuFd,
		IIOW(kvmSetOneReg, unsafe.Sizeof(OneRegister{})),
		uintptr(unsafe.Pointer(&reg)))

	return err
}
