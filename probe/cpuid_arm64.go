package probe

import "errors"

// ErrCPUIDUnsupported indicates CPUID probing is an x86-only concept
// (there is no arm64 equivalent instruction/ioctl); this stub exists
// only so callers that unconditionally invoke CPUID() (e.g. the -probe
// CLI flag) still build on arm64.
var ErrCPUIDUnsupported = errors.New("CPUID probing is not supported on arm64")

// CPUID is not supported on arm64. See probe/cpuid_amd64.go for the
// x86 implementation.
func CPUID() error {
	return ErrCPUIDUnsupported
}
