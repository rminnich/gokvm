package pvh

import "github.com/bobuhiro11/gokvm/kvm"

// For GDT details see arch/x86/include/asm/segment.h and the Intel
// SDM's description of the 64-bit segment descriptor format.
//
// Bit layout of a segment descriptor (bit 0 is the LSB):
//
//	 0-15: limit[0:15]
//	16-39: base[0:23]
//	40-43: type (4 bits)
//	   44: S
//	45-46: DPL (2 bits)
//	   47: P
//	48-51: limit[16:19]
//	   52: AVL
//	   53: L
//	   54: DB
//	   55: G
//	56-63: base[24:31]
const (
	gdtLimitLoBits = 16 // width of limit[0:15]
	gdtBaseLoBits  = 24 // width of base[0:23]

	gdtTypeShift    = 40
	gdtSShift       = 44
	gdtDPLShift     = 45
	gdtPShift       = 47
	gdtLimitHiShift = 48
	gdtAVLShift     = 52
	gdtLShift       = 53
	gdtDBShift      = 54
	gdtGShift       = 55
	gdtBaseHiShift  = 56

	gdtFlagMask = 0x1 // width of single-bit fields: G, DB, L, AVL, P, S
	gdtDPLMask  = 0x3
	gdtTypeMask = 0xF

	gdtLimitLoMask = 0x0000FFFF
	gdtLimitHiMask = 0x000F0000
	gdtBaseLoMask  = 0x00FFFFFF
	gdtBaseHiMask  = 0xFF000000
	gdtFlagsMask   = 0x0000F0FF

	// gdtGranularityShift is how much a limit is scaled by when the G
	// (granularity) flag is set (4 KiB pages).
	gdtGranularityShift = 12

	// gdtSelectorStride is the size, in bytes, of one GDT entry, used to
	// convert a table index into a segment selector.
	gdtSelectorStride = 8
)

// u8 truncates v to its low 8 bits. Used for single-byte GDT
// descriptor fields (type, DPL, and the single-bit flags), which are
// always small by construction (masked to their field width below).
func u8(v uint64) uint8 {
	return uint8(v) //nolint:gosec // masked to the field's bit width above
}

// u32 truncates v to its low 32 bits. Used for the reconstructed
// 20-bit segment limit below, which always fits well within 32 bits.
func u32(v uint64) uint32 {
	return uint32(v) //nolint:gosec // masked to 20 bits, see getLimit
}

func GdtEntry(flags uint16, base uint32, limit uint32) uint64 {
	return (uint64(base)&gdtBaseHiMask)<<(gdtBaseHiShift-gdtBaseLoBits) |
		(uint64(flags)&gdtFlagsMask)<<gdtTypeShift |
		(uint64(limit)&gdtLimitHiMask)<<(gdtLimitHiShift-gdtLimitLoBits) |
		(uint64(base)&gdtBaseLoMask)<<gdtLimitLoBits |
		(uint64(limit) & gdtLimitLoMask)
}

func getBase(entry uint64) uint64 {
	// baseHiShift undoes the (gdtBaseHiShift-gdtBaseLoBits) shift
	// GdtEntry applies when encoding the high byte of base.
	const baseHiShift = gdtBaseHiShift - gdtBaseLoBits

	return ((entry & (uint64(gdtBaseHiMask) << baseHiShift)) >> baseHiShift) |
		((entry & (uint64(gdtBaseLoMask) << gdtLimitLoBits)) >> gdtLimitLoBits)
}

func getG(entry uint64) uint8 {
	return u8((entry >> gdtGShift) & gdtFlagMask)
}

func getDB(entry uint64) uint8 {
	return u8((entry >> gdtDBShift) & gdtFlagMask)
}

func getL(entry uint64) uint8 {
	return u8((entry >> gdtLShift) & gdtFlagMask)
}

func getAVL(entry uint64) uint8 {
	return u8((entry >> gdtAVLShift) & gdtFlagMask)
}

func getP(entry uint64) uint8 {
	return u8((entry >> gdtPShift) & gdtFlagMask)
}

func getDPL(entry uint64) uint8 {
	return u8((entry >> gdtDPLShift) & gdtDPLMask)
}

func getS(entry uint64) uint8 {
	return u8((entry >> gdtSShift) & gdtFlagMask)
}

func getType(entry uint64) uint8 {
	return u8((entry >> gdtTypeShift) & gdtTypeMask)
}

// Extract the segment limit from the GDT segment descriptor.
//
// In a segment descriptor, the limit field is 20 bits, so it can directly describe
// a range from 0 to 0xFFFFF (1MByte). When G flag is set (4-KByte page granularity) it
// scales the value in the limit field by a factor of 2^12 (4Kbytes), making the effective
// limit range from 0xFFF (4 KBytes) to 0xFFFF_FFFF (4 GBytes).
//
// However, the limit field in the VMCS definition is a 32 bit field, and the limit value is not
// automatically scaled using the G flag. This means that for a desired range of 4GB for a
// given segment, its limit must be specified as 0xFFFF_FFFF. Therefore the method of obtaining
// the limit from the GDT entry is not sufficient, since it only provides 20 bits when 32 bits
// are necessary. Fortunately, we can check if the G flag is set when extracting the limit since
// the full GDT entry is passed as an argument, and perform the scaling of the limit value to
// return the full 32 bit value.
//
// The scaling mentioned above is required when using PVH boot, since the guest boots in protected
// (32-bit) mode and must be able to access the entire 32-bit address space. It does not cause issues
// for the case of direct boot to 64-bit (long) mode, since in 64-bit mode the processor does not
// perform runtime limit checking on code or data segments.
func getLimit(entry uint64) uint32 {
	// limitHiShift undoes the (gdtLimitHiShift-gdtLimitLoBits) shift
	// GdtEntry applies when encoding the high nibble of limit.
	const limitHiShift = gdtLimitHiShift - gdtLimitLoBits

	l := u32(((entry & (uint64(gdtLimitHiMask) << limitHiShift)) >> limitHiShift) | (entry & gdtLimitLoMask))
	g := getG(entry)

	switch g {
	case 0:
		return l
	default:
		return (l << gdtGranularityShift) | gdtLimitLoMask
	}
}

func SegmentFromGDT(entry uint64, tableIndex uint8) kvm.Segment {
	var unused uint8

	u := getP(entry)

	switch u {
	case 0:
		unused = 1
	default:
		unused = 0
	}

	return kvm.Segment{
		Base:     getBase(entry),
		Limit:    getLimit(entry),
		Selector: uint16(tableIndex) * gdtSelectorStride,
		Typ:      getType(entry),
		Present:  getP(entry),
		DPL:      getDPL(entry),
		DB:       getDB(entry),
		S:        getS(entry),
		L:        getL(entry),
		G:        getG(entry),
		AVL:      getAVL(entry),
		Unusable: unused,
	}
}
