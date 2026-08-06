package probe

import (
	"fmt"
	"runtime"
)

// kvmCapabilitiesImpl is set to the real implementation on amd64, nil otherwise.
var kvmCapabilitiesImpl func() error

// cpuidImpl is set to the real implementation on amd64, nil otherwise.
var cpuidImpl func() error

// KVMCapabilities probes the system for KVM capabilities.
// On non-amd64 architectures this returns an unsupported error.
func KVMCapabilities() error {
	if kvmCapabilitiesImpl == nil {
		return fmt.Errorf("KVMCapabilities: not supported on %s", runtime.GOARCH)
	}

	return kvmCapabilitiesImpl()
}

// CPUID calls KVM_GET_SUPPORTED_CPUID and prints the result.
// On non-amd64 architectures this returns an unsupported error.
func CPUID() error {
	if cpuidImpl == nil {
		return fmt.Errorf("CPUID: not supported on %s", runtime.GOARCH)
	}

	return cpuidImpl()
}
