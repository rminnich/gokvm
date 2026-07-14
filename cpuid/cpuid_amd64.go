package cpuid

func cpuid_low(arg1, arg2 uint32) (eax, ebx, ecx, edx uint32) // implemented in cpuid_amd64.s

func CPUID(leaf uint32) (uint32, uint32, uint32, uint32) {
	return cpuid_low(leaf, 0)
}
