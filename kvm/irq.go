//nolint:dupl
package kvm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"unsafe"
)

// irqLevel defines an IRQ as Level? Not sure.
type irqLevel struct {
	IRQ   uint32
	Level uint32
}

// IRQLines sets the interrupt line for an IRQ.
func IRQLineStatus(vmFd uintptr, irq, level uint32) error {
	irqLev := irqLevel{
		IRQ:   irq,
		Level: level,
	}
	_, err := Ioctl(vmFd,
		IIOWR(kvmIRQLineStatus, unsafe.Sizeof(irqLevel{})),
		uintptr(unsafe.Pointer(&irqLev)))

	return err
}

// CreateIRQChip creates an IRQ device (chip) to which to attach interrupts?
func CreateIRQChip(vmFd uintptr) error {
	_, err := Ioctl(vmFd, IIO(kvmCreateIRQChip), 0)

	return err
}

type IRQChip struct {
	ChipID uint32
	_      uint32
	Chip   [512]byte
}

// GetIRQChip reads the state of a kernel interrupt controller created with
// KVM_CREATE_IRQCHIP into a buffer provided by the caller.
func GetIRQChip(vmFd uintptr, irqc *IRQChip) error {
	_, err := Ioctl(vmFd,
		IIOWR(kvmGetIRQChip, unsafe.Sizeof(IRQChip{})),
		uintptr(unsafe.Pointer(irqc)))

	return err
}

// SetIRQChip sets the state of a kernel interrupt controller created with
// KVM_CREATE_IRQCHIP from a buffer provided by the caller.
func SetIRQChip(vmFd uintptr, irqc *IRQChip) error {
	_, err := Ioctl(vmFd,
		IIOR(kvmSetIRQChip, unsafe.Sizeof(IRQChip{})), uintptr(unsafe.Pointer(irqc)))

	return err
}

type IRQRoutingIRQChip struct {
	IRQChip uint32
	Pin     uint32
}

type IRQRoutingEntry struct {
	GSI   uint32
	Type  uint32
	Flags uint32
	_     uint32
	IRQRoutingIRQChip
}

type IRQRouting struct {
	Nr      uint32
	Flags   uint32
	Entries []IRQRoutingEntry
}

func (r *IRQRouting) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, r.Nr); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, r.Flags); err != nil {
		return nil, err
	}

	for _, entry := range r.Entries {
		if err := binary.Write(&buf, binary.LittleEndian, entry); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func NewIRQRouting(data []byte) (*IRQRouting, error) {
	r := IRQRouting{}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, data); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	if err := binary.Read(&buf, binary.LittleEndian, &r.Nr); err != nil {
		return nil, err
	}

	if err := binary.Read(&buf, binary.LittleEndian, &r.Flags); err != nil {
		return nil, err
	}

	r.Entries = make([]IRQRoutingEntry, r.Nr)

	if err := binary.Read(&buf, binary.LittleEndian, &r.Entries); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	return &r, nil
}

// SetGSIRouting sets the GSI routing table entries, overwriting any previously set entries.
func SetGSIRouting(vmFd uintptr, irqR *IRQRouting) error {
	data, err := irqR.Bytes()
	if err != nil {
		return err
	}

	_, err = Ioctl(vmFd,
		IIOW(kvmSetGSIRouting, unsafe.Sizeof(irqR)),
		uintptr(unsafe.Pointer(&data[0])))

	return err
}
