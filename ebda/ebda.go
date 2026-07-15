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
	mpEntryTypeProcessor = 0

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

		mpcCPU [maxVCPUs]mpcCPU
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

func newMPCTable(nCPUs int) (*mpcTable, error) {
	m := &mpcTable{}
	m.signature = mpcTableSignature
	m.length = uint16(unsafe.Sizeof(mpcTable{})) // this field must contain the size of entries.
	m.spec = 4
	m.lapic = apicAddr(0)
	m.OEMId = [8]byte{0x47, 0x4F, 0x4B, 0x56, 0x4D, 0x00, 0x00, 0x00} // "GOKVM   "
	m.oemCount = maxVCPUs                                             // This must be the number of entries

	if nCPUs > maxVCPUs {
		return nil, errorVCPUNumExceed
	}

	var err error

	for i := range nCPUs {
		m.mpcCPU[i] = *newMPCCpu(i)
	}

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
