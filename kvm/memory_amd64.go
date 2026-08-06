package kvm

import "unsafe"

// SetTSSAddr sets the Task Segment Selector for a vm (x86 only).
func SetTSSAddr(vmFd uintptr, addr uint32) error {
	_, err := Ioctl(vmFd, IIO(kvmSetTSSAddr), uintptr(addr))

	return err
}

// SetIdentityMapAddr sets the address of a 4k-sized-page for a vm (x86 only).
func SetIdentityMapAddr(vmFd uintptr, addr uint32) error {
	_, err := Ioctl(vmFd, IIOW(kvmSetIdentityMapAddr, 8), uintptr(unsafe.Pointer(&addr)))

	return err
}
