package vmm

// VMInfo holds architecture-defined information about a running VMM.
// Each architecture's Runner.Info() populates the fields it supports.
type VMInfo struct {
	// Arch is the GOARCH string of the VMM implementation.
	Arch string
	// NCPUs is the number of virtual CPUs.
	NCPUs int
	// MemSize is the guest memory size in bytes.
	MemSize int
	// KernelPath is the path of the kernel image that was loaded.
	KernelPath string
	// CPUIDEntries holds the x86 CPUID entries (amd64 only; nil on other arches).
	CPUIDEntries []CPUIDEntry
}

// CPUIDEntry describes a single x86 CPUID leaf (amd64 only).
type CPUIDEntry struct {
	Function uint32
	Index    uint32
	Eax      uint32
	Ebx      uint32
	Ecx      uint32
	Edx      uint32
}

// Runner is the interface that a VMM implementation must satisfy.
// Each architecture provides its own implementation via a _GOARCH.go file.
type Runner interface {
	// Init creates and configures the virtual machine hardware.
	Init() error
	// Setup loads the kernel, initrd, and parameters into guest memory.
	Setup() error
	// Boot starts the vCPUs and the console I/O loop; blocks until exit.
	Boot() error
	// Close stops the VMM, interrupting any blocked vCPU ioctls.
	Close() error
	// Info returns architecture-defined information about this VMM.
	Info() VMInfo
}

// Config holds the parameters that describe a virtual machine.
type Config struct {
	Debug      bool
	Dev        string
	Kernel     string
	Initrd     string
	Params     string
	TapIfName  string
	Disk       string
	NCPUs      int
	MemSize    int
	TraceCount int
	Resume     string // path to state file for -R resume
	SavePath   string // path to write state on save
}

// VMM wraps a Runner and a Config.
// New is defined in each architecture-specific file (e.g. vmm_amd64.go).
type VMM struct {
	Runner
	Config
}
