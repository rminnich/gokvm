package kvm

import "unsafe"

// ARM64-only ioctl numbers, from the Linux KVM UAPI
// (arch/arm64/include/uapi/asm/kvm.h). See kvm.go for the
// architecture-generic ioctl numbers.
const (
	// kvmARMVCPUInit is a vcpu ioctl. It must be called once per vcpu,
	// after CreateVCPU and before the vcpu can be Run.
	kvmARMVCPUInit = 0xae

	// kvmARMPreferredTarget is a vm ioctl that returns the target CPU
	// type to pass to VCPUInit.
	kvmARMPreferredTarget = 0xaf
)

// ARM target CPU types, as returned by PreferredTarget and accepted by
// VCPUInit.
const (
	TargetAEMV8        = 0
	TargetFoundationV8 = 1
	TargetCortexA57    = 2
	TargetXgenePotenza = 3
	TargetCortexA53    = 4
	TargetGenericV8    = 5
)

// numFeatures is the length of the VCPUInit.Features array
// (KVM_VCPU_MAX_FEATURES in the Linux kernel sources).
const numFeatures = 7

// VCPUInit mirrors the arm64 struct kvm_vcpu_init. Target should
// normally be obtained via PreferredTarget. Features is a bitmask
// array of optional CPU features to enable (e.g. PSCI, pointer auth).
type VCPUInit struct {
	Target   uint32
	Features [numFeatures]uint32
}

// PreferredTarget returns the preferred target CPU type for this host,
// to be passed (unmodified, or with Features set) to VCPUInit.
func PreferredTarget(vmFd uintptr) (*VCPUInit, error) {
	init := &VCPUInit{}
	_, err := Ioctl(vmFd,
		IIOR(kvmARMPreferredTarget, unsafe.Sizeof(VCPUInit{})),
		uintptr(unsafe.Pointer(init)))

	return init, err
}

// VCPUInitialize initializes a vcpu with the given target/features. It
// must be called once per vcpu before the vcpu can be Run.
func VCPUInitialize(vcpuFd uintptr, init *VCPUInit) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmARMVCPUInit, unsafe.Sizeof(VCPUInit{})),
		uintptr(unsafe.Pointer(init)))

	return err
}
