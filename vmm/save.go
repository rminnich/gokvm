package vmm

// VCPUSave holds the architecture-defined register state for a single vCPU,
// sufficient to resume that vCPU. The Regs field holds an arch-specific
// struct (e.g. *machine.AMD64State on amd64, nil on unsupported arches).
type VCPUSave struct {
	// CPU is the vCPU index this state was captured from.
	CPU int
	// Regs holds the arch-specific register state.
	// On amd64 this is a *machine.AMD64State.
	// On other architectures it is nil.
	Regs interface{}
}

// Save holds everything needed to resume a stopped VMM:
// guest RAM and per-vCPU register state.
type Save struct {
	// Arch is the GOARCH of the VMM that created this save.
	Arch string
	// Mem is a snapshot of guest RAM.
	Mem []byte
	// VCPUs holds per-vCPU register state, one entry per CPU.
	VCPUs []VCPUSave
}
