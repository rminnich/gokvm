package iodev

import (
	"time"
)

const (
	indexMask   = uint8(0x7F)
	indexOffset = uint64(0x70)
	dataOffset  = uint64(0x71)
	dataLen     = uint64(128)

	// CMOS RAM register addresses used by Read/Write below.
	cmosRegSeconds = 0x00
	cmosRegMinutes = 0x02
	cmosRegHours   = 0x04
	cmosRegWeekday = 0x06
	cmosRegDay     = 0x07
	cmosRegMonth   = 0x08
	cmosRegYear    = 0x09
	cmosRegStatusA = 0x0A
	cmosRegStatusD = 0x0D
	cmosRegCentury = 0x32

	// Status Register A: divider bits 6:4 = 0b010 selects the
	// 32.768 kHz time base; bit 7 = Update-In-Progress.
	cmosStatusAFreq32kHz = 1 << 5
	cmosStatusAUIPClear  = 0 << 7
	cmosStatusA          = cmosStatusAFreq32kHz | cmosStatusAUIPClear

	// Status Register D: bit 7 indicates the CMOS RAM/battery is valid.
	cmosStatusDValidRAM = 1 << 7

	// bcdNibbleShift is the bit width of a BCD nibble.
	bcdNibbleShift = 4

	// yearCenturyDivisor extracts the century from a 4-digit year.
	yearCenturyDivisor = 100

	// timeCenturyBase mirrors the traditional C `tm_year + 1900`
	// century arithmetic, kept as-is from the original implementation.
	timeCenturyBase = 1900

	// defaultExtMemKB is the CMOS-reported extended memory size (in KB,
	// above 1 MB), assuming a fixed 3 GB of RAM for now.
	defaultExtMemKB = 0xBC00

	// ioPortIndex/ioPortSize are this device's IO port address and size.
	ioPortIndex = 0x70
	ioPortSize  = 0x2

	// extMemHiByteShift extracts the high byte of the 16-bit extended
	// memory size stored across CMOS registers 0x34/0x35.
	extMemHiByteShift = 8
)

type CMOS struct {
	Index uint8
	Data  []uint8
}

// u8 truncates v to its low 8 bits. Used only where v is known by
// construction to be a small value (a byte-sized field of a hardware
// register, or a wall-clock second/minute/hour component).
func u8(v int) uint8 {
	return uint8(v) //nolint:gosec // bounded by construction, see call sites
}

func NewCMOS(memBelow4G, memAbove4G uint64) *CMOS {
	cmos := &CMOS{
		Index: 0,
		Data:  make([]uint8, dataLen),
	}

	// We assume 3G RAM at all times for now.....
	extMem := uint16(defaultExtMemKB)

	cmos.Data[0x34] = u8(int(extMem))
	cmos.Data[0x35] = u8(int(extMem >> extMemHiByteShift))

	// Only valid for PVH boot of firmware with 3G RAM fixed.....
	cmos.Data[0x5b] = 0
	cmos.Data[0x5c] = 0
	cmos.Data[0x5d] = 0

	return cmos
}

func (c *CMOS) Read(base uint64, data []byte) error {
	if len(data) != 1 {
		return errDataLenInvalid
	}

	var d uint8

	// Reading CMOS RAM also requires two steps:
	// 1. OUT to port hex 70 with the CMOS address that is to be read from.
	// 2. IN from port hex 71, and the data read is returned in the AL register.
	// Ref: http://bitsavers.trailing-edge.com/pdf/ibm/pc/at/1502494_PC_AT_Technical_Reference_Mar84.pdf
	switch base {
	case indexOffset:
		data[0] = c.Index
	case dataOffset:
		dt := time.Now()
		secs := dt.Second()
		minute := dt.Minute()
		hour := dt.Hour()
		weekd := dt.Weekday()
		day := dt.Day()
		month := dt.Month()
		year := dt.Year()

		switch c.Index {
		case cmosRegSeconds:
			d = toBCD(u8(secs))
		case cmosRegMinutes:
			d = toBCD(u8(minute))
		case cmosRegHours:
			d = toBCD(u8(hour))
		case cmosRegWeekday:
			d = toBCD(u8(int(weekd)))
		case cmosRegDay:
			d = toBCD(u8(day))
		case cmosRegMonth:
			d = toBCD(u8(int(month)))
		case cmosRegYear:
			d = toBCD(u8(year % yearCenturyDivisor))
		case cmosRegStatusA:
			d = cmosStatusA // 32kHz Clock and we assume no update in progress
		case cmosRegStatusD:
			d = cmosStatusDValidRAM
		case cmosRegCentury:
			d = toBCD(u8((year + timeCenturyBase) / yearCenturyDivisor))
		default:
			d = c.Data[c.Index&indexMask]
		}

		data[0] = d
	}

	return nil
}

func (c *CMOS) Write(base uint64, data []byte) error {
	if len(data) != 1 {
		return errDataLenInvalid
	}

	// Writing to CMOS RAM involves two steps:
	// 1. OUT to port hex 70 with the CMOS address that will be written to.
	// 2. OUT to port hex 71 with the data to be written.
	// Ref: http://bitsavers.trailing-edge.com/pdf/ibm/pc/at/1502494_PC_AT_Technical_Reference_Mar84.pdf
	switch base {
	case indexOffset:
		c.Index = data[0]
	case dataOffset:
		if c.Index == 0x8F && data[0] == 0 {
			// CMOS reset - we ignore for now
		} else {
			c.Data[c.Index&indexMask] = data[0]
		}
	}

	return nil
}

func toBCD(v uint8) uint8 {
	const (
		bcdDivisor    = 100
		bcdOnesModulo = 10
	)

	return ((v / bcdDivisor) << bcdNibbleShift) | (v % bcdOnesModulo)
}

func (c *CMOS) IOPort() uint64 {
	return ioPortIndex
}

func (c *CMOS) Size() uint64 {
	return ioPortSize
}
