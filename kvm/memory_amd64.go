package kvm

import "unsafe"

// SetTSSAddr sets the Task Segment Selector for a vm.
func SetTSSAddr(vmFd uintptr, addr uint32) error {
	_, err := Ioctl(vmFd, IIO(kvmSetTSSAddr), uintptr(addr))

	return err
}

// SetIdentityMapAddr sets the address of a 4k-sized-page for a vm.
func SetIdentityMapAddr(vmFd uintptr, addr uint32) error {
	// The kernel's KVM_SET_IDENTITY_MAP_ADDR argument is a __u64,
	// regardless of the (32-bit) address value passed here.
	const identityMapAddrArgSize = 8

	_, err := Ioctl(vmFd, IIOW(kvmSetIdentityMapAddr, identityMapAddrArgSize), uintptr(unsafe.Pointer(&addr)))

	return err
}
