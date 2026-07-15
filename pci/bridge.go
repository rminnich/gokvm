package pci

import "errors"

var ErrIONotPermit = errors.New("IO is not permitted for PCI bridge")

// bridgeDeviceID/bridgeVendorID identify this emulated device as an
// Intel (0x8086) host bridge (0x0d57).
const (
	bridgeDeviceID = 0x0d57
	bridgeVendorID = 0x8086

	// bridgeBAR0Size is the (unused) BAR0 IO-space size advertised by
	// this bridge.
	bridgeBAR0Size = 0x10
)

type bridge struct{}

func (br bridge) GetDeviceHeader() DeviceHeader {
	return DeviceHeader{
		DeviceID:      bridgeDeviceID,
		VendorID:      bridgeVendorID,
		HeaderType:    1,
		SubsystemID:   0,
		InterruptLine: 0,
		InterruptPin:  0,
		BAR:           [6]uint32{},
		Command:       0,
	}
}

func (br bridge) Read(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (br bridge) Write(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (br bridge) IOPort() uint64 {
	return 0
}

func (br bridge) Size() uint64 {
	return bridgeBAR0Size
}

func NewBridge() Device {
	return &bridge{}
}
