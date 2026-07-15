// Package machine implements a KVM-based virtual machine.
package machine

import "errors"

// MinMemSize is the smallest guest memory size New will accept.
const MinMemSize = 1 << 25

// ErrZeroSizeKernel indicates a 0-byte kernel file was given to load.
var ErrZeroSizeKernel = errors.New("kernel is 0 bytes")

// ErrBadVA indicates a bad virtual (or, for load addresses, physical)
// address was used.
var ErrBadVA = errors.New("bad virtual address")

// ErrMemTooSmall indicates the requested memory size is too small.
var ErrMemTooSmall = errors.New("mem request must be at least 1<<20")

// ErrMachineStopped is returned by RunOnce when Machine.Close has been
// called.
var ErrMachineStopped = errors.New("machine stopped")
