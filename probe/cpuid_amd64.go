package probe

import (
	"fmt"
	"os"

	"github.com/bobuhiro11/gokvm/kvm"
)

// CPUID call 'KVM_GET_SUPPORTED_CPUID' and print the result.
func CPUID() error {
	// maxCPUIDEntries is the number of kvm.CPUIDEntry2 slots to allocate
	// for KVM_GET_SUPPORTED_CPUID; the kernel returns E2BIG if more
	// entries than this are available, but 100 is comfortably above any
	// real CPU's supported-leaf count.
	const maxCPUIDEntries = 100

	kvmFile, err := os.Open("/dev/kvm")
	if err != nil {
		return err
	}
	defer kvmFile.Close()

	kvmfd := kvmFile.Fd()

	cpuid := kvm.CPUID{
		Nent:    maxCPUIDEntries,
		Entries: make([]kvm.CPUIDEntry2, maxCPUIDEntries),
	}

	if err := kvm.GetSupportedCPUID(kvmfd, &cpuid); err != nil {
		return err
	}

	for _, e := range cpuid.Entries {
		fmt.Printf("0x%08x 0x%02x: eax=0x%08x ebx=0x%08x ecx=0x%08x edx=0x%08x (flag:%x)\n",
			e.Function, e.Index, e.Eax, e.Ebx, e.Ecx, e.Edx, e.Flags)
	}

	return nil
}
