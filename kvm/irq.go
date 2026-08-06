package kvm

import (
	"fmt"
	"runtime"
	"unsafe"
)

// irqLevel defines an IRQ level pair used by IRQLineStatus.
type irqLevel struct {
	IRQ   uint32
	Level uint32
}

// kvmIRQLineStatusNr is the KVM ioctl number for IRQ line status (x86).
const kvmIRQLineStatusNr = 0x67

// IRQLineStatus sets the level of an IRQ line.
// On non-amd64 architectures this returns an unsupported error at runtime.
func IRQLineStatus(vmFd uintptr, irq, level uint32) error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("IRQLineStatus: not supported on %s", runtime.GOARCH)
	}

	irqLev := irqLevel{IRQ: irq, Level: level}
	_, err := Ioctl(vmFd, IIOWR(kvmIRQLineStatusNr, unsafe.Sizeof(irqLevel{})), uintptr(unsafe.Pointer(&irqLev)))

	return err
}
