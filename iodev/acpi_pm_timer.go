package iodev

import (
	"encoding/binary"
	"time"
)

type ACPIPMTimer struct {
	Start time.Time
}

const (
	pmTimerFreqHz  uint64 = 3_579_545
	nanosPerSecond uint64 = 1_000_000_000

	// pmTimerDataWidth is the width, in bytes, of the ACPI PM timer
	// counter register.
	pmTimerDataWidth = 4

	pmTimerIOPort = 0x608
	pmTimerSize   = 0x4
)

// u32 truncates a signed 64-bit timer counter to its low 32 bits (the
// ACPI PM timer counter is intentionally free-running and wraps at
// 2^32).
func u32(v int64) uint32 {
	return uint32(v) //nolint:gosec // intentional 32-bit wraparound counter
}

func NewACPIPMTimer() *ACPIPMTimer {
	return &ACPIPMTimer{
		Start: time.Now(),
	}
}

func (a *ACPIPMTimer) Read(base uint64, data []byte) error {
	if len(data) != pmTimerDataWidth {
		return errDataLenInvalid
	}

	since := time.Since(a.Start)
	nanos := since.Nanoseconds()
	counter := (nanos * int64(pmTimerFreqHz)) / int64(nanosPerSecond)
	counter32 := u32(counter)
	counterbyte := make([]byte, pmTimerDataWidth)

	binary.LittleEndian.PutUint32(counterbyte, counter32)

	copy(data[0:], counterbyte)

	return nil
}

func (a *ACPIPMTimer) Write(base uint64, data []byte) error {
	return nil
}

func (a *ACPIPMTimer) IOPort() uint64 {
	return pmTimerIOPort
}

func (a *ACPIPMTimer) Size() uint64 {
	return pmTimerSize
}
