package ebda

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/bobuhiro11/gokvm/bootparam"
)

const (
	maxVCPUs = 64

	// Use the default physical address for the APIC.
	// https://github.com/torvalds/linux/blob/c5c17547b778975b3d83a73c8d84e8fb5ecf3ba5/arch/x86/include/asm/apicdef.h#L13
	apicDefaultPhysBase = 0xfee00000

	// The physical address for APIC has a stride for each apic ID.
	// https://github.com/kvmtool/kvmtool/blob/415f92c33a227c02f6719d4594af6fad10f07abf/include/kvm/apic.h#L9
	apicBaseAddrStep = 0x00400000

	// ioAPICPhysBase is KVM's fixed in-kernel I/O APIC MMIO base address
	// (IOAPIC_DEFAULT_BASE_ADDRESS in arch/x86/kvm/ioapic.h), the same
	// default used by real PC hardware and by QEMU. It is not
	// configurable via any KVM ioctl, so we must advertise this exact
	// value in the MP table for the guest to find it.
	ioAPICPhysBase = 0xfec00000

	// topByteShift/thirdByteShift/secondByteShift position the first
	// three characters of a 4-byte, big-endian-style ASCII signature
	// (see mpfIntelSignature/mpcTableSignature below) within a uint32.
	topByteShift    = 24
	thirdByteShift  = 16
	secondByteShift = 8

	mpfIntelSignature = (('_' << topByteShift) | ('P' << thirdByteShift) | ('M' << secondByteShift) | '_')
	mpcTableSignature = (('P' << topByteShift) | ('M' << thirdByteShift) | ('C' << secondByteShift) | 'P')

	// see Table 4-3. Base MP Configuration Table Entry Types in Intel MP Configuration
	// https://pdos.csail.mit.edu/6.828/2014/readings/ia32/MPspec.pdf
	mpEntryTypeProcessor         = 0
	mpEntryTypeBus               = 1
	mpEntryTypeIOAPIC            = 2
	mpEntryTypeIOInterruptAssign = 3

	// see Table 4-4. Processor Entry Fields in Intel MP Configuration
	// https://pdos.csail.mit.edu/6.828/2014/readings/ia32/MPspec.pdf
	cpuFlagEnabled = 1
	cpuFlagBP      = 2

	cpuStepping    = uint32(0x600)
	cpuFeatureAPIC = uint32(0x200)
	cpuFeatureFPU  = uint32(0x001)

	// cpuSteppingShift places cpuStepping into the correct bit position
	// within mpcCPU.sig (the CPUID signature field).
	cpuSteppingShift = 16

	mpAPICVersion = uint8(0x14)

	// mpfIntelPhysPtrOffset is the offset, from the start of the EBDA,
	// of the MP Configuration Table pointed to by the MP Floating
	// Pointer Structure.
	mpfIntelPhysPtrOffset = 0x40

	// byteMask masks a value down to its low 8 bits; also used, XORed,
	// to compute the one's-complement byte needed to make an MP table
	// checksum sum to 0.
	byteMask = 0xff

	// mpBusIDISA is the (arbitrary but conventional) bus ID used for the
	// single ISA bus this MP table describes.
	mpBusIDISA = 0

	// mpIOAPICID is the (arbitrary but conventional) ID assigned to the
	// single I/O APIC this MP table describes.
	mpIOAPICID = 2

	// mpIOAPICVersion is the I/O APIC version register value for a
	// standard 82093AA-compatible I/O APIC (matches KVM's emulated one).
	mpIOAPICVersion = 0x11

	// mpIOAPICFlagEnable marks the I/O APIC entry as enabled.
	mpIOAPICFlagEnable = 1

	// mpIntTypeINT is a standard vectored interrupt (as opposed to NMI,
	// SMI, or ExtINT) in an I/O/Local Interrupt Assignment entry.
	mpIntTypeINT = 0

	// mpIntFlagBusDefault means "use the bus's default polarity/trigger
	// mode" (edge-triggered, active-high, for the ISA bus).
	mpIntFlagBusDefault = 0

	// numISAIRQs is the number of legacy ISA IRQ lines (0-15) this MP
	// table identity-maps to I/O APIC pins 0-15.
	numISAIRQs = 16
)

var errorVCPUNumExceed = fmt.Errorf("the number of vCPUs must be less than or equal to %d", maxVCPUs)

// u8 truncates v to its low 8 bits. Used for the one's-complement MP
// table checksum (always masked to a byte) and other intentionally
// byte-sized derived values below.
func u8(v uint32) uint8 {
	return uint8(v) //nolint:gosec // intentional truncation to a checksum/ID byte
}

// u8FromInt truncates a small, known-non-negative int (an APIC/CPU
// index, always well below 256) to uint8.
func u8FromInt(v int) uint8 {
	return uint8(v) //nolint:gosec // v is a small CPU index, always < maxVCPUs
}

type (
	// Extended BIOS Data Area (EBDA).
	EBDA struct {
		// padding
		// It must be aligned with 16 bytes and its size must be less than 1KB.
		// https://github.com/torvalds/linux/blob/2f111a6fd5b5297b4e92f53798ca086f7c7d33a4/arch/x86/kernel/mpparse.c#L597
		_        [16 * 3]uint8
		mpfIntel mpfIntel
		mpcTable mpcTable
	}

	// Intel MP Floating Pointer Structure
	// ported from https://github.com/torvalds/linux/blob/5bfc75d92/arch/x86/include/asm/mpspec_def.h#L22-L33
	mpfIntel struct {
		signature     uint32
		physPtr       uint32
		length        uint8
		specification uint8
		checkSum      uint8
		_             uint8 // feature1
		_             uint8 // feature2
		_             uint8 // feature3
		_             uint8 // feature4
		_             uint8 // feature5
	}

	// MP Configuration Table Header
	// ported from https://github.com/torvalds/linux/blob/5bfc75d92/arch/x86/include/asm/mpspec_def.h#L37-L49
	mpcTable struct {
		signature uint32
		length    uint16
		spec      uint8
		checkSum  uint8
		OEMId     [8]uint8
		ProdID    [12]uint8
		_         uint32 // oemPtr
		_         uint16 // oemSize
		oemCount  uint16
		lapic     uint32 // Local APIC addresss must be set.
		_         uint32 // reserved

		mpcCPU         [maxVCPUs]mpcCPU
		mpcBus         mpcBus
		mpcIOAPIC      mpcIOAPIC
		mpcIOInterrupt [numISAIRQs]mpcIOInterrupt
	}

	// MP Bus Entry (type 1): identifies one system bus (here, the
	// single ISA bus every I/O interrupt assignment below refers to).
	// See Table 4-5 in the Intel MP Configuration spec.
	mpcBus struct {
		typ     uint8
		busID   uint8
		busType [6]uint8 // ASCII bus type string, e.g. "ISA   "
	}

	// MP I/O APIC Entry (type 2): describes one I/O APIC. See Table 4-6
	// in the Intel MP Configuration spec.
	mpcIOAPIC struct {
		typ      uint8
		apicID   uint8
		apicVer  uint8
		flags    uint8
		apicAddr uint32
	}

	// MP I/O Interrupt Assignment Entry (type 3): routes one source bus
	// IRQ line to one I/O APIC input pin. See Table 4-7 in the Intel MP
	// Configuration spec.
	mpcIOInterrupt struct {
		typ        uint8
		intType    uint8
		flags      uint16
		srcBusID   uint8
		srcBusIRQ  uint8
		dstAPICID  uint8
		dstAPICINT uint8
	}
)

func (e *EBDA) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, e); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func New(nCPUs int) (*EBDA, error) {
	e := &EBDA{}

	mpfIntel, err := newMPFIntel()
	if err != nil {
		return e, err
	}

	e.mpfIntel = *mpfIntel

	mpcTable, err := newMPCTable(nCPUs)
	if err != nil {
		return e, err
	}

	e.mpcTable = *mpcTable

	return e, nil
}

func newMPFIntel() (*mpfIntel, error) {
	m := &mpfIntel{}
	m.signature = mpfIntelSignature
	m.length = 1 // this must be 1
	m.specification = 4
	m.physPtr = bootparam.EBDAStart + mpfIntelPhysPtrOffset

	var err error

	m.checkSum, err = m.calcCheckSum()
	if err != nil {
		return m, err
	}

	m.checkSum ^= u8(byteMask)
	m.checkSum++

	return m, nil
}

func (m *mpfIntel) calcCheckSum() (uint8, error) {
	bytes, err := m.bytes()
	if err != nil {
		return 0, err
	}

	tmp := uint32(0)
	for _, b := range bytes {
		tmp += uint32(b)
	}

	return u8(tmp & byteMask), nil
}

func (m *mpfIntel) bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, m); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func apicAddr(apic uint32) uint32 {
	return apicDefaultPhysBase + apic*apicBaseAddrStep
}

// newMPCBus returns the single ISA bus entry every I/O interrupt
// assignment below refers to as its source bus.
func newMPCBus() mpcBus {
	return mpcBus{
		typ:     mpEntryTypeBus,
		busID:   mpBusIDISA,
		busType: [6]uint8{'I', 'S', 'A', ' ', ' ', ' '},
	}
}

// newMPCIOAPIC returns the single I/O APIC entry, describing KVM's
// fixed in-kernel I/O APIC.
func newMPCIOAPIC() mpcIOAPIC {
	return mpcIOAPIC{
		typ:      mpEntryTypeIOAPIC,
		apicID:   mpIOAPICID,
		apicVer:  mpIOAPICVersion,
		flags:    mpIOAPICFlagEnable,
		apicAddr: ioAPICPhysBase,
	}
}

// newMPCIOInterrupts returns one I/O Interrupt Assignment entry per
// legacy ISA IRQ line (0-15), identity-mapping each to the same-numbered
// I/O APIC input pin, using the bus's default (edge, active-high)
// polarity and trigger mode.
func newMPCIOInterrupts() [numISAIRQs]mpcIOInterrupt {
	var entries [numISAIRQs]mpcIOInterrupt

	for irq := range numISAIRQs {
		entries[irq] = mpcIOInterrupt{
			typ:        mpEntryTypeIOInterruptAssign,
			intType:    mpIntTypeINT,
			flags:      mpIntFlagBusDefault,
			srcBusID:   mpBusIDISA,
			srcBusIRQ:  u8FromInt(irq),
			dstAPICID:  mpIOAPICID,
			dstAPICINT: u8FromInt(irq),
		}
	}

	return entries
}

func newMPCTable(nCPUs int) (*mpcTable, error) {
	m := &mpcTable{}
	m.signature = mpcTableSignature
	m.length = uint16(unsafe.Sizeof(mpcTable{})) // this field must contain the size of entries.
	m.spec = 4
	m.lapic = apicAddr(0)
	m.OEMId = [8]byte{0x47, 0x4F, 0x4B, 0x56, 0x4D, 0x00, 0x00, 0x00} // "GOKVM   "
	// This must be the number of entries: maxVCPUs processor entries
	// (unused slots are zeroed/disabled), plus 1 bus, 1 I/O APIC, and
	// numISAIRQs I/O interrupt assignment entries.
	m.oemCount = maxVCPUs + 1 + 1 + numISAIRQs

	if nCPUs > maxVCPUs {
		return nil, errorVCPUNumExceed
	}

	var err error

	for i := range nCPUs {
		m.mpcCPU[i] = *newMPCCpu(i)
	}

	m.mpcBus = newMPCBus()
	m.mpcIOAPIC = newMPCIOAPIC()
	m.mpcIOInterrupt = newMPCIOInterrupts()

	m.checkSum, err = m.calcCheckSum()
	if err != nil {
		return m, err
	}

	m.checkSum ^= u8(byteMask)
	m.checkSum++

	return m, nil
}

func (m *mpcTable) calcCheckSum() (uint8, error) {
	bytes, err := m.bytes()
	if err != nil {
		return 0, err
	}

	tmp := uint32(0)
	for _, b := range bytes {
		tmp += uint32(b)
	}

	return u8(tmp & byteMask), nil
}

func (m *mpcTable) bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, m); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type mpcCPU struct {
	typ         uint8
	apicID      uint8 // Local APIC number
	apicVer     uint8
	cpuFlag     uint8
	sig         uint32
	featureFlag uint32
	_           [2]uint32 // reserved
}

func newMPCCpu(i int) *mpcCPU {
	m := &mpcCPU{}

	f := uint8(cpuFlagEnabled)

	if i == 0 { // CPU 0 is Boot Processor(BP), every other is Application Processor(AP)
		f |= cpuFlagBP
	}

	m.typ = mpEntryTypeProcessor
	m.apicID = u8FromInt(i)
	m.apicVer = mpAPICVersion
	m.cpuFlag = f
	m.sig = (cpuStepping << cpuSteppingShift)
	m.featureFlag = cpuFeatureAPIC | cpuFeatureFPU

	return m
}
