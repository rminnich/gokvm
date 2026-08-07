package vmm
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
	// Info returns a Save struct containing guest RAM and per-vCPU register
	// state captured at the time the VMM was stopped. Returns nil if the
	// VMM has not been stopped or state is not available.
	Info() *Save
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
	NoPoison   bool   // skip memory poisoning for faster boot
}

// VMM wraps a Runner and a Config.
// New is defined in each architecture-specific file (e.g. vmm_amd64.go).
type VMM struct {
	Runner
	Config
}
