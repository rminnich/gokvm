package virtio

import "log"

// Trace enables verbose per-request/per-packet tracing for virtio-net and
// virtio-blk devices (queue setup, kicks, and rx/tx activity). It is off by
// default; the `boot` subcommand's `-vtrace` flag enables it.
//
// CLI flag before any VM is created; threading it through every virtio
// constructor and device method would add a parameter to nearly every
// call in this package for no benefit.
//
//nolint:gochecknoglobals // process-wide runtime toggle set once from a
var Trace bool

// tracef logs a formatted trace message when Trace is enabled.
func tracef(format string, v ...any) {
	if Trace {
		log.Printf(format, v...)
	}
}

// traceln logs a trace message when Trace is enabled.
func traceln(v ...any) {
	if Trace {
		log.Println(v...)
	}
}

const (
	// The number of free descriptors in virt queue must exceed
	// MAX_SKB_FRAGS (16). Otherwise, packet transmission from
	// the guest to the host will be stopped.
	//
	// refs https://github.com/torvalds/linux/blob/5859a2b/drivers/net/virtio_net.c#L1754
	QueueSize = 32

	// pageSize is the guest page size used to align the virtqueue PFN.
	pageSize = 4096

	// virtioVendorID is the PCI vendor ID used by legacy virtio devices.
	virtioVendorID = 0x1AF4

	// Legacy virtio-pci IO-port register offsets (see commonHeader
	// below and https://wiki.osdev.org/Virtio#Legacy_Interface).
	regQueuePFN    = 8
	regQueueSelect = 14
	regQueueNotify = 16
	regISR         = 19

	// isrClear is the cleared (no interrupt pending) ISR status value.
	isrClear = 0x0
)

type IRQInjector interface {
	InjectVirtioNetIRQ() error
	InjectVirtioBlkIRQ() error
}

type commonHeader struct {
	_        uint32 // hostFeatures
	_        uint32 // guestFeatures
	_        uint32 // queuePFN
	queueNUM uint16
	queueSEL uint16
	_        uint16 // queueNotify
	_        uint8  // status
	isr      uint8
}

// refs: https://wiki.osdev.org/Virtio#Virtual_Queue_Descriptor
type VirtQueue struct {
	DescTable [QueueSize]struct {
		Addr  uint64
		Len   uint32
		Flags uint16
		Next  uint16
	}

	AvailRing struct {
		Flags     uint16
		Idx       uint16
		Ring      [QueueSize]uint16
		UsedEvent uint16
	}

	// padding for 4096 byte alignment
	_ [pageSize - ((16*QueueSize + 6 + 2*QueueSize) % pageSize)]uint8

	UsedRing struct {
		Flags uint16
		Idx   uint16
		Ring  [QueueSize]struct {
			Idx uint32
			Len uint32
		}
		availEvent uint16
	}
}

// The helpers below centralize narrowing/widening integer conversions
// that are safe by construction in this package (IO-port offsets are
// always small, guest-supplied PFN/queue-index values fit their
// target width, and disk offsets/sizes never approach the signed/
// unsigned boundary for any realistic disk image), so that gosec's
// G115 (integer overflow conversion) is addressed in one place rather
// than with a nolint comment at every call site.

// toInt narrows a uint64 IO-port-relative offset to int.
func toInt(v uint64) int {
	return int(v) //nolint:gosec // v is always a small IO-port offset
}

// u16 narrows a uint64 guest-supplied value (e.g. a virtqueue select
// index) to uint16.
func u16(v uint64) uint16 {
	return uint16(v) //nolint:gosec // v is always a small queue index
}

// u32 narrows a uint64 guest-physical page-frame-number-derived
// address to uint32.
func u32(v uint64) uint32 {
	return uint32(v) //nolint:gosec // v is a PFN*pageSize address within guest memory
}

// i64 widens a uint64 byte offset/size (always well below 2^63) to int64.
func i64(v uint64) int64 {
	return int64(v) //nolint:gosec // v is a disk offset/size, always < 2^63
}

// u64 widens a non-negative int64 (a file size from os.FileInfo.Size())
// to uint64.
func u64(v int64) uint64 {
	return uint64(v) //nolint:gosec // v is a file size, always non-negative
}
