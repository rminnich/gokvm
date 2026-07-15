package pvh

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"io"

	"github.com/bobuhiro11/gokvm/kvm"
)

const (
	xenHVMstartMagicValue uint32 = 0x336ec578
	xenELFNotePhys32Entry uint32 = 18
	pvhNoteStrSz          uint32 = 4
	elfNoteSize                  = 12

	// elfNoteFieldSize is the size, in bytes, of each of the three
	// fixed-width fields (namesz, descsz, type) in an ELF note header.
	elfNoteFieldSize = 4
)

var (
	errAlign            = errors.New("alignment is not a power of 2")
	errPVHEntryNotFound = errors.New("no pvh entry found")
)

type HVMStartInfo struct {
	Magic         uint32
	Version       uint32
	Flags         uint32
	NrModules     uint32
	ModlistPAddr  uint64
	CmdLinePAddr  uint64
	RSDPPAddr     uint64
	MemMapPAddr   uint64
	MemMapEntries uint32
	_             uint32
}

func NewStartInfo(rsdpPAddr, cmdLinePAddr uint64) *HVMStartInfo {
	return &HVMStartInfo{
		Magic:        xenHVMstartMagicValue,
		Version:      1,
		NrModules:    0,
		CmdLinePAddr: cmdLinePAddr,
		RSDPPAddr:    rsdpPAddr,
		MemMapPAddr:  PVHMemMapStart,
	}
}

func (h *HVMStartInfo) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	for _, item := range []interface{}{
		h.Magic,
		h.Version,
		h.Flags,
		h.NrModules,
		h.ModlistPAddr,
		h.CmdLinePAddr,
		h.RSDPPAddr,
		h.MemMapPAddr,
		h.MemMapEntries,
		uint32(0),
	} {
		if err := binary.Write(&buf, binary.LittleEndian, item); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

type HVMModListEntry struct {
	Addr        uint64
	Size        uint64
	CmdLineAddr uint64
	_           uint64
}

func NewModListEntry(addr, size, cmdaddr uint64) *HVMModListEntry {
	return &HVMModListEntry{
		Addr:        addr,
		Size:        size,
		CmdLineAddr: cmdaddr,
	}
}

func (h *HVMModListEntry) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	for _, item := range []interface{}{
		h.Addr,
		h.Size,
		h.CmdLineAddr,
		uint64(0),
	} {
		if err := binary.Write(&buf, binary.LittleEndian, item); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

type HVMMemMapTableEntry struct {
	Addr uint64
	Size uint64
	Type uint32
	_    uint32
}

func NewMemMapTableEntry(addr, size uint64, t uint32) *HVMMemMapTableEntry {
	return &HVMMemMapTableEntry{
		Addr: addr,
		Size: size,
		Type: t,
	}
}

func (h *HVMMemMapTableEntry) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	for _, item := range []interface{}{
		h.Addr,
		h.Size,
		h.Type,
		uint32(0),
	} {
		if err := binary.Write(&buf, binary.LittleEndian, item); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

type GDT [4]uint64

// GDT entry access-byte flags for the fixed code/data/TSS segments
// CreateGDT builds (see the Intel SDM's segment descriptor format).
const (
	codeSegFlags = 0xc09b
	dataSegFlags = 0xc093
	tssSegFlags  = 0x008b
	tssSegLimit  = 0x67

	// codeDataSegLimit is the 32-bit (4 GiB, with G=1) limit given to
	// the code and data segments.
	codeDataSegLimit = 0xffffffff

	// GDT table indices for the code/data/TSS segments below.
	codeSegIndex = 1
	dataSegIndex = 2
	tssSegIndex  = 3

	// idtLimit is the (placeholder) IDT limit InitSRegs sets.
	idtLimit = 8

	// EFLAGS-style bit positions cleared in EFER below (VM=virtual-8086
	// mode, IF=interrupt enable, TF=trap/single-step).
	vmBit = 17
	ifBit = 9
	tfBit = 8
)

func CreateGDT() GDT {
	var gdtTable GDT

	gdtTable[0] = GdtEntry(0, 0, 0)                                      // NULL
	gdtTable[codeSegIndex] = GdtEntry(codeSegFlags, 0, codeDataSegLimit) // Code
	gdtTable[dataSegIndex] = GdtEntry(dataSegFlags, 0, codeDataSegLimit) // DATA
	gdtTable[tssSegIndex] = GdtEntry(tssSegFlags, 0, tssSegLimit)        // TSS

	return gdtTable
}

func InitSRegs(vcpuFd uintptr, gdttable GDT) error {
	codeseg := SegmentFromGDT(gdttable[codeSegIndex], codeSegIndex)
	dataseg := SegmentFromGDT(gdttable[dataSegIndex], dataSegIndex)
	tssseg := SegmentFromGDT(gdttable[tssSegIndex], tssSegIndex)

	// We need to write this to ....maybe create this config earlier.
	gdt := kvm.Descriptor{
		Base:  BootGDTStart,
		Limit: uint16(len(gdttable)*gdtSelectorStride) - 1, // 4 entries of 64bit (8byte) per entry
	}

	idt := kvm.Descriptor{
		Base:  BootIDTStart,
		Limit: idtLimit,
	}

	sregs, err := kvm.GetSregs(vcpuFd)
	if err != nil {
		return err
	}

	sregs.GDT = gdt
	sregs.IDT = idt

	sregs.CS = codeseg
	sregs.DS = dataseg
	sregs.ES = dataseg
	sregs.FS = dataseg
	sregs.GS = dataseg
	sregs.SS = dataseg
	sregs.TR = tssseg

	sregs.EFER |= (0 << vmBit) | (0 << ifBit) | (0 << tfBit) // VM=0, IF=0, TF=0

	sregs.CR0 = 0x1
	sregs.CR4 = 0x0

	return kvm.SetSregs(vcpuFd, sregs)
}

func (gdt GDT) Bytes() []byte {
	bytes := make([]byte, binary.Size(gdt))

	for i, entry := range gdt {
		binary.LittleEndian.PutUint64(bytes[i*binary.Size(entry):], entry)
	}

	return bytes
}

func InitRegs(vcpuFd uintptr, bootIP uint64) error {
	regs, err := kvm.GetRegs(vcpuFd)
	if err != nil {
		return err
	}

	regs.RFLAGS = 0x2
	regs.RBX = PVHInfoStart
	regs.RIP = bootIP

	return kvm.SetRegs(vcpuFd, regs)
}

type elfNote struct {
	NameSize uint32
	DescSize uint32
	Type     uint32
}

// toI64 widens a uint64 ELF file offset/size (always well below 2^63
// for any realistic kernel image) to int64.
func toI64(v uint64) int64 {
	return int64(v) //nolint:gosec // v is an ELF file offset/size, always < 2^63
}

// iToI64 widens a non-negative int (a byte count or accumulated read
// size, always small) to int64.
func iToI64(v int) int64 {
	return int64(v) //nolint:gosec // v is a small, non-negative byte count
}

// toInt narrows a uint64 ELF program-header field (always small for a
// realistic kernel image) to int.
func toInt(v uint64) int {
	return int(v) //nolint:gosec // v is a small ELF size field
}

func ParsePVHEntry(fwimg io.ReaderAt, phdr *elf.Prog) (uint32, error) {
	node := elfNote{}
	off := toI64(phdr.Off)
	readSize := 0

	for readSize < toInt(phdr.Filesz) {
		nodeByte := make([]byte, elfNoteSize)

		n, err := fwimg.ReadAt(nodeByte, off)
		if err != nil {
			return 0, err
		}

		readSize += n
		off += iToI64(n)

		nsb := make([]byte, elfNoteFieldSize)
		dsb := make([]byte, elfNoteFieldSize)
		tsb := make([]byte, elfNoteFieldSize)

		copy(nsb, nodeByte[:3])
		copy(dsb, nodeByte[4:7])
		copy(tsb, nodeByte[8:])

		node.NameSize = binary.LittleEndian.Uint32(nsb)
		node.DescSize = binary.LittleEndian.Uint32(dsb)
		node.Type = binary.LittleEndian.Uint32(tsb)

		if node.Type == xenELFNotePhys32Entry && node.NameSize == pvhNoteStrSz {
			buf := make([]byte, pvhNoteStrSz)

			n, err := fwimg.ReadAt(buf, off)
			if err != nil {
				return 0, err
			}

			off += iToI64(n)
			// Check the String
			if bytes.Equal(buf, []byte{'X', 'e', 'n', '\000'}) {
				break
			}
		}

		nameAlign, err := alignUp(uint64(node.NameSize))
		if err != nil {
			return 0, err
		}

		descAlign, err := alignUp(uint64(node.DescSize))
		if err != nil {
			return 0, err
		}

		readSize += toInt(nameAlign)
		readSize += toInt(descAlign)
		off = toI64(phdr.Off) + iToI64(readSize)
	}

	if readSize >= toInt(phdr.Filesz) {
		// No PVH entry found. Return
		return 0, errPVHEntryNotFound
	}

	// off is the value we need to add aligned namesize - PVH_NOTE_STR_SZ
	nameAlign, err := alignUp(uint64(node.NameSize))
	if err != nil {
		return 0, err
	}

	off += toI64(nameAlign) - toI64(uint64(pvhNoteStrSz))
	pvhAddrByte := make([]byte, elfNoteFieldSize) // address is 4 byte/32-bit

	if _, err := fwimg.ReadAt(pvhAddrByte, off); err != nil {
		return 0, err
	}

	retAddr := binary.LittleEndian.Uint32(pvhAddrByte)

	return retAddr, nil
}

func alignUp(addr uint64) (uint64, error) {
	align := uint64(elfNoteFieldSize)
	if !isPowerOf2(align) {
		return addr, errAlign
	}

	alignMask := align - 1

	if addr&alignMask == 0 {
		return addr, nil
	}

	return (addr | alignMask) + 1, nil
}

func isPowerOf2(n uint64) bool {
	if n == 0 {
		return true
	}

	return (n & (n - 1)) == 0
}

func CheckPVH(kern io.ReaderAt) (bool, error) {
	elfkern, err := elf.NewFile(kern)
	if err != nil {
		return false, nil //nolint:nilerr
	}
	defer elfkern.Close()

	for _, prog := range elfkern.Progs {
		note := elfNote{}
		off := toI64(prog.Off)
		readSize := 0

		for readSize < toInt(prog.Filesz) {
			noteByte := make([]byte, elfNoteSize)

			n, err := kern.ReadAt(noteByte, off)
			if err != nil {
				return false, err
			}

			readSize += n
			off += iToI64(n)

			nsb := make([]byte, elfNoteFieldSize)
			dsb := make([]byte, elfNoteFieldSize)
			tsb := make([]byte, elfNoteFieldSize)

			copy(nsb, noteByte[:3])
			copy(dsb, noteByte[4:7])
			copy(tsb, noteByte[8:])

			note.NameSize = binary.LittleEndian.Uint32(nsb)
			note.DescSize = binary.LittleEndian.Uint32(dsb)
			note.Type = binary.LittleEndian.Uint32(tsb)

			if note.Type == xenELFNotePhys32Entry && note.NameSize == pvhNoteStrSz {
				buf := make([]byte, pvhNoteStrSz)

				_, err := kern.ReadAt(buf, off)
				if err != nil {
					return false, err
				}

				if bytes.Equal(buf, []byte{'X', 'e', 'n', '\000'}) {
					return true, nil
				}
			}

			nameAlign, err := alignUp(uint64(note.NameSize))
			if err != nil {
				return false, err
			}

			descAlign, err := alignUp(uint64(note.DescSize))
			if err != nil {
				return false, err
			}

			readSize += toInt(nameAlign)
			readSize += toInt(descAlign)
			off = toI64(prog.Off) + iToI64(readSize)
		}
	}

	return false, nil
}
