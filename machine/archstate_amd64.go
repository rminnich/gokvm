package machine

import (
	"log"
	"unsafe"

	"github.com/bobuhiro11/gokvm/kvm"
	"golang.org/x/arch/x86/x86asm"
)

// AMD64State holds the complete CPU state needed to resume an amd64 VM.
// GPRs are indexed by x86asm.Reg constants (RAX, RBX, ..., RIP).
// Special registers (control, segment, descriptor) are in Sregs.
// RFLAGS is stored separately as it has no x86asm.Reg constant.
type AMD64State struct {
	// GPR maps x86asm register constants to their 64-bit values.
	// Populated from kvm.Regs: RAX, RBX, RCX, RDX, RSI, RDI,
	// RSP, RBP, R8-R15, RIP.
	GPR map[x86asm.Reg]uint64

	// RFLAGS holds the CPU flags register.
	RFLAGS uint64

	// Sregs holds the x86 special/control registers: segment descriptors,
	// CR0, CR2, CR3, CR4, CR8, EFER, APIC base, and interrupt bitmap.
	Sregs kvm.Sregs
}

// GetAMD64State returns the captured AMD64State, or nil if not yet captured.
func (m *Machine) GetAMD64State() *AMD64State {
	p := m.LoadArchState()
	if p == nil {
		return nil
	}

	return (*AMD64State)(p)
}

// captureArchState snapshots the register state of the given vCPU into
// an AMD64State and stores it on the Machine. Only the first vCPU to stop
// captures state; subsequent calls are no-ops if state is already set.
func (m *Machine) captureArchState(cpu int) {
	// Only capture once — first vCPU to stop wins.
	if m.LoadArchState() != nil {
		return
	}

	fd, err := m.CPUToFD(cpu)
	if err != nil {
		log.Printf("captureArchState: CPUToFD(%d): %v", cpu, err)
		return
	}

	regs, err := kvm.GetRegs(fd)
	if err != nil {
		log.Printf("captureArchState: GetRegs cpu %d: %v", cpu, err)
		return
	}

	sregs, err := kvm.GetSregs(fd)
	if err != nil {
		log.Printf("captureArchState: GetSregs cpu %d: %v", cpu, err)
		return
	}

	state := &AMD64State{
		GPR: map[x86asm.Reg]uint64{
			x86asm.RAX: regs.RAX,
			x86asm.RBX: regs.RBX,
			x86asm.RCX: regs.RCX,
			x86asm.RDX: regs.RDX,
			x86asm.RSI: regs.RSI,
			x86asm.RDI: regs.RDI,
			x86asm.RSP: regs.RSP,
			x86asm.RBP: regs.RBP,
			x86asm.R8:  regs.R8,
			x86asm.R9:  regs.R9,
			x86asm.R10: regs.R10,
			x86asm.R11: regs.R11,
			x86asm.R12: regs.R12,
			x86asm.R13: regs.R13,
			x86asm.R14: regs.R14,
			x86asm.R15: regs.R15,
			x86asm.RIP: regs.RIP,
		},
		RFLAGS: regs.RFLAGS,
		Sregs:  *sregs,
	}

	m.StoreArchState(unsafe.Pointer(state))
}
