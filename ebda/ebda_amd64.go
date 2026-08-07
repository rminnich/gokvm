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

	mpfIntelSignature = (('_' << 24) | ('P' << 16) | ('M' << 8) | '_')
	mpcTableSignature = (('P' << 24) | ('M' << 16) | ('C' << 8) | 'P')

	// see Table 4-3. Base MP Configuration Table Entry Types in Intel MP Configuration
	// https://pdos.csail.mit.edu/6.828/2014/readings/ia32/MPspec.pdf
	mpEntryTypeProcessor = 0
	mpEntryTypeBus       = 1
	mpEntryTypeIOAPIC    = 2
	mpEntryTypeIOIntr    = 3

	// see Table 4-4. Processor Entry Fields in Intel MP Configuration
	// https://pdos.csail.mit.edu/6.828/2014/readings/ia32/MPspec.pdf
	cpuFlagEnabled = 1
	cpuFlagBP      = 2

	cpuStepping    = uint32(0x600)
	cpuFeatureAPIC = uint32(0x200)
	cpuFeatureFPU  = uint32(0x001)

	mpAPICVersion = uint8(0x14)

	// IOAPIC address and ID
	// refs: https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/x86/mptable.c
	ioAPICAddr = 0xfec00000
	ioAPICID   = 0

	// ISA bus ID
	isaBusID = 0

	// Number of ISA IRQs to map
	numISAIRQs = 16
)

var errorVCPUNumExceed = fmt.Errorf("the number of vCPUs must be less than or equal to %d", maxVCPUs)

type (
	// Extended BIOS Data Area (EBDA).
	EBDA struct {
		// padding
		// It must be aligned with 16 bytes and its size must be less than 1KB.
		// https://github.com/torvalds/linux/blob/2f111a6fd5b5297b4e92f53798ca086f7c7d33a4/arch/x86/kernel/mpparse.c#L597
		padding [16 * 3]uint8

		mpfIntel mpfIntel

		// mpcTableBytes holds the serialised MP Configuration Table including
		// CPU, bus, IOAPIC, and IRQ source entries. Variable length.
		mpcTableBytes []byte
	}

	// Intel MP Floating Pointer Structure
	// ported from https://github.com/torvalds/linux/blob/5bfc75d92/arch/x86/include/asm/mpspec_def.h#L22-L33
	mpfIntel struct {
		signature     uint32
		physPtr       uint32
		length        uint8
		specification uint8
		checkSum      uint8
		_ uint8 // feature1
		_ uint8 // feature2
		_ uint8 // feature3
		_ uint8 // feature4
		_ uint8 // feature5
	}

	// MP Configuration Table Header (fixed part only)
	mpcTableHeader struct {
		signature uint32
		length    uint16
		spec      uint8
		checkSum  uint8
		OEMId     [8]uint8
		ProdID    [12]uint8
		_         uint32 // oemPtr
		_         uint16 // oemSize
		oemCount  uint16
		lapic     uint32
		_         uint32 // reserved
	}

	mpcCPU struct {
		typ         uint8
		apicID      uint8
		apicVer     uint8
		cpuFlag     uint8
		sig         uint32
		featureFlag uint32
		_           [2]uint32 // reserved
	}

	mpcBus struct {
		typ   uint8
		busID uint8
		name  [6]uint8
	}

	mpcIOAPIC struct {
		typ      uint8
		apicID   uint8
		apicVer  uint8
		flags    uint8
		apicAddr uint32
	}

	mpcIntsrc struct {
		typ      uint8
		irqType  uint8
		irqFlag  uint16
		srcBus   uint8
		srcBusIRQ uint8
		dstAPIC  uint8
		dstIRQ   uint8
	}
)

func (e *EBDA) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, e.padding); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, e.mpfIntel); err != nil {
		return nil, err
	}

	buf.Write(e.mpcTableBytes)

	return buf.Bytes(), nil
}

func New(nCPUs int) (*EBDA, error) {
	e := &EBDA{}

	mpf, err := newMPFIntel()
	if err != nil {
		return e, err
	}

	e.mpfIntel = *mpf

	tableBytes, err := buildMPCTable(nCPUs)
	if err != nil {
		return e, err
	}

	e.mpcTableBytes = tableBytes

	return e, nil
}

func newMPFIntel() (*mpfIntel, error) {
	m := &mpfIntel{}
	m.signature = mpfIntelSignature
	m.length = 1 // this must be 1
	m.specification = 4
	m.physPtr = bootparam.EBDAStart + 0x40

	var err error

	m.checkSum, err = calcCheckSum(m)
	if err != nil {
		return m, err
	}

	m.checkSum ^= uint8(0xff)
	m.checkSum++

	return m, nil
}

func calcCheckSum(v interface{}) (uint8, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
		return 0, err
	}

	tmp := uint32(0)
	for _, b := range buf.Bytes() {
		tmp += uint32(b)
	}

	return uint8(tmp & 0xff), nil
}

func apicAddr(apic uint32) uint32 {
	return apicDefaultPhysBase + apic*apicBaseAddrStep
}

// buildMPCTable constructs the full MP Configuration Table as a byte slice,
// including CPU, bus, IOAPIC, and I/O interrupt source entries.
// Having explicit IRQ entries prevents the Linux kernel from printing
// "BIOS bug, no explicit IRQ entries" and using a slow fallback path.
func buildMPCTable(nCPUs int) ([]byte, error) {
	if nCPUs > maxVCPUs {
		return nil, errorVCPUNumExceed
	}

	// Build entry bytes first so we know the total length.
	var entries bytes.Buffer

	// CPU entries
	for i := 0; i < nCPUs; i++ {
		cpu := newMPCCpu(i)
		if err := binary.Write(&entries, binary.LittleEndian, cpu); err != nil {
			return nil, err
		}
	}

	// Bus entry: ISA bus
	bus := mpcBus{
		typ:   mpEntryTypeBus,
		busID: isaBusID,
		name:  [6]uint8{'I', 'S', 'A', ' ', ' ', ' '},
	}
	if err := binary.Write(&entries, binary.LittleEndian, bus); err != nil {
		return nil, err
	}

	// IOAPIC entry
	ioapic := mpcIOAPIC{
		typ:      mpEntryTypeIOAPIC,
		apicID:   ioAPICID,
		apicVer:  mpAPICVersion,
		flags:    1, // enabled
		apicAddr: ioAPICAddr,
	}
	if err := binary.Write(&entries, binary.LittleEndian, ioapic); err != nil {
		return nil, err
	}

	// I/O interrupt source entries: one per ISA IRQ (0-15).
	// Each ISA IRQ maps 1:1 to IOAPIC pin.
	// refs: https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/x86/mptable.c
	for irq := 0; irq < numISAIRQs; irq++ {
		intsrc := mpcIntsrc{
			typ:       mpEntryTypeIOIntr,
			irqType:   0, // INT
			irqFlag:   0, // default polarity and trigger
			srcBus:    isaBusID,
			srcBusIRQ: uint8(irq),
			dstAPIC:   ioAPICID,
			dstIRQ:    uint8(irq),
		}
		if err := binary.Write(&entries, binary.LittleEndian, intsrc); err != nil {
			return nil, err
		}
	}

	// Build the header with the correct total length.
	headerSize := int(unsafe.Sizeof(mpcTableHeader{}))
	totalLength := uint16(headerSize + entries.Len())

	// oemCount = total number of entries
	oemCount := uint16(nCPUs + 1 + 1 + numISAIRQs) // CPUs + bus + ioapic + irqs

	hdr := mpcTableHeader{
		signature: mpcTableSignature,
		length:    totalLength,
		spec:      4,
		lapic:     apicAddr(0),
		OEMId:     [8]uint8{'G', 'O', 'K', 'V', 'M', ' ', ' ', ' '},
		ProdID:    [12]uint8{},
		oemCount:  oemCount,
	}

	// Compute checksum over header + entries with checkSum=0.
	var full bytes.Buffer

	if err := binary.Write(&full, binary.LittleEndian, hdr); err != nil {
		return nil, err
	}

	full.Write(entries.Bytes())

	var sum uint8
	for _, b := range full.Bytes() {
		sum += b
	}

	hdr.checkSum = uint8(0x100 - int(sum))

	// Re-serialise with correct checksum.
	var out bytes.Buffer

	if err := binary.Write(&out, binary.LittleEndian, hdr); err != nil {
		return nil, err
	}

	out.Write(entries.Bytes())

	return out.Bytes(), nil
}

func newMPCCpu(i int) *mpcCPU {
	m := &mpcCPU{}

	f := uint8(cpuFlagEnabled)

	if i == 0 { // CPU 0 is Boot Processor(BP), every other is Application Processor(AP)
		f |= cpuFlagBP
	}

	m.typ = mpEntryTypeProcessor
	m.apicID = uint8(i)
	m.apicVer = mpAPICVersion
	m.cpuFlag = f
	m.sig = (cpuStepping << 16)
	m.featureFlag = cpuFeatureAPIC | cpuFeatureFPU

	return m
}
