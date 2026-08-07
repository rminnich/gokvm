package vmm

import "github.com/bobuhiro11/gokvm/machine"

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

// Save holds everything needed to resume a stopped VMM.
// Machine-level state (guest RAM) is captured once in Machine and
// embedded here; per-vCPU register state is in VCPUs.
type Save struct {
	// Arch is the GOARCH of the VMM that created this save.
	Arch string
	// Machine holds the machine-level save (guest RAM).
	// Captured once — shared across all VMMs on this machine.
	Machine *machine.Save
	// VCPUs holds per-vCPU register state, one entry per CPU.
	VCPUs []VCPUSave
}
