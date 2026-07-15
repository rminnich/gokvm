package pci

import (
	"bytes"
	"encoding/binary"
)

// Configuration Space Access Mechanism #1
//
// refs
// https://wiki.osdev.org/PCI
// http://www2.comp.ufscar.br/~helio/boot-int/pci.html
type address uint32

// Bit-layout of the 32-bit PCI CONFIG_ADDRESS register (Configuration
// Space Access Mechanism #1): enable bit, bus/device/function numbers,
// and register offset.
const (
	registerOffsetMask = 0xfc

	functionNumberShift = 8
	functionNumberMask  = 0x7

	deviceNumberShift = 11
	deviceNumberMask  = 0x1f

	busNumberShift = 16
	busNumberMask  = 0xff

	enableBitShift = 31
	enableBitMask  = 0x1
)

func (a address) getRegisterOffset() uint32 {
	return uint32(a) & registerOffsetMask
}

func (a address) getFunctionNumber() uint32 {
	return (uint32(a) >> functionNumberShift) & functionNumberMask
}

func (a address) getDeviceNumber() uint32 {
	return (uint32(a) >> deviceNumberShift) & deviceNumberMask
}

func (a address) getBusNumber() uint32 {
	return (uint32(a) >> busNumberShift) & busNumberMask
}

func (a address) isEnable() bool {
	return ((uint32(a) >> enableBitShift) | enableBitMask) == enableBitMask
}

// interface for a PCI device.
type Device interface {
	GetDeviceHeader() DeviceHeader
	Read(port uint64, data []byte) error
	Write(port uint64, data []byte) error

	// IO port range for this PCI device.
	// This range corresponds to IO Range in BAR0.
	IOPort() uint64
	Size() uint64
}

// configRegWidth is the width, in bytes, of a PCI configuration-space
// register access via the 0xCF8/0xCFC IO ports.
const configRegWidth = 4

// u32 truncates v to its low 32 bits. PCI config-space register
// offsets and addresses are inherently 32-bit values carried in a
// uint64 IO-port value; this truncation is intentional, not an
// overflow bug.
func u32(v uint64) uint32 {
	return uint32(v) //nolint:gosec // intentional truncation to a 32-bit PCI register/address
}

// u8 truncates v to its low 8 bits, used when serializing a register
// value one byte at a time.
func u8(v uint64) uint8 {
	return uint8(v) //nolint:gosec // intentional truncation, one byte of a wire register value
}

type DeviceHeader struct {
	VendorID      uint16
	DeviceID      uint16
	Command       uint16
	_             uint16   // status
	_             uint8    // revisonID
	_             [3]uint8 // classCode
	_             uint8    // cacheLineSize
	_             uint8    // latencyTimer
	HeaderType    uint8
	_             uint8 // bist
	BAR           [6]uint32
	_             uint32 // cardbusCISPointer
	_             uint16 // subsystemVendorID
	SubsystemID   uint16
	_             uint32   // expansionROMBaseAddress
	_             uint8    // capabilitiesPointer
	_             [7]uint8 // reserved
	InterruptLine uint8
	InterruptPin  uint8
	_             uint8 // minGnt
	_             uint8 // maxLat
}

func (h DeviceHeader) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type PCI struct {
	addr        address
	isBAR0Probe bool
	Devices     []Device
}

func New(devices ...Device) *PCI {
	return &PCI{Devices: devices}
}

// ioPortConfData is the base IO port address (0xCFC) for the PCI
// Configuration Space Access Mechanism #1 CONFIG_DATA register.
const ioPortConfData = 0xCFC

func (p *PCI) PciConfDataIn(port uint64, values []byte) error {
	// offset can be obtained from many source as below:
	//        (address from IO port 0xcf8) & 0xfc + (IO port address for Data) - 0xCFC
	// see pci_conf1_read in linux/arch/x86/pci/direct.c for more detail.
	offset := int(p.addr.getRegisterOffset() + u32(port-ioPortConfData))

	if !p.addr.isEnable() {
		return nil
	}

	if p.addr.getBusNumber() != 0 {
		return nil
	}

	if p.addr.getFunctionNumber() != 0 {
		return nil
	}

	slot := int(p.addr.getDeviceNumber())

	if slot >= len(p.Devices) {
		return nil
	}

	// Probing BAR0 Size
	if bar := offset/configRegWidth - configRegWidth; bar == 0 && p.isBAR0Probe {
		size := p.Devices[slot].Size()
		copy(values[:4], NumToBytes(SizeToBits(size)))

		p.isBAR0Probe = false

		return nil
	}

	b, err := p.Devices[slot].GetDeviceHeader().Bytes()
	if err != nil {
		return err
	}

	l := len(values)
	copy(values[:l], b[offset:offset+l])

	return nil
}

func (p *PCI) PciConfDataOut(port uint64, values []byte) error {
	offset := int(p.addr.getRegisterOffset() + u32(port-ioPortConfData))

	if !p.addr.isEnable() {
		return nil
	}

	if p.addr.getBusNumber() != 0 {
		return nil
	}

	if p.addr.getFunctionNumber() != 0 {
		return nil
	}

	slot := int(p.addr.getDeviceNumber())

	if slot >= len(p.Devices) {
		return nil
	}

	// Probing BAR0 Size
	if bar := offset/configRegWidth - configRegWidth; bar == 0 && BytesToNum(values) == 0xffffffff {
		p.isBAR0Probe = true

		return nil
	}

	return nil
}

func (p *PCI) PciConfAddrIn(port uint64, values []byte) error {
	if len(values) != configRegWidth {
		return nil
	}

	copy(values[:configRegWidth], NumToBytes(uint32(p.addr)))

	return nil
}

func (p *PCI) PciConfAddrOut(port uint64, values []byte) error {
	if len(values) != configRegWidth {
		return nil
	}

	p.addr = address(u32(BytesToNum(values)))

	return nil
}

func SizeToBits(size uint64) uint32 {
	if size == 0 {
		return 0
	}

	// BAR-size probing: after writing all 1s to a BAR and reading it
	// back, the cleared low bits indicate the BAR's size. The expected
	// mask is the 32-bit two's-complement negation of size.
	return u32(-size)
}

// byteBits is the number of bits in a byte, used when packing/unpacking
// a register value one byte at a time.
const byteBits = 8

func BytesToNum(bytes []byte) uint64 {
	res := uint64(0)

	for i, x := range bytes {
		res |= uint64(x) << (i * byteBits)
	}

	return res
}

func NumToBytes(x interface{}) []byte {
	res := []byte{}
	l := 0
	y := uint64(0)

	switch v := x.(type) {
	case uint8:
		l = 1
		y = uint64(v)
	case uint16:
		l = 2
		y = uint64(v)
	case uint32:
		l = 4
		y = uint64(v)
	case uint64:
		l = 8
		y = v
	default:
		return []byte{}
	}

	for range l {
		res = append(res, u8(y))
		y >>= 8
	}

	return res
}
