package machine

import (
	"encoding/gob"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/bobuhiro11/gokvm/kvm"
)

// VMState holds the serializable snapshot of a running VM.
type VMState struct {
	Mem       []byte
	Regs      []*kvm.Regs
	Sregs     []*kvm.Sregs
	SerialIER byte
	SerialLCR byte
}

// Save snapshots guest RAM and all vCPU register state to path.
// It sets ImmediateExit=1 on all vCPUs to ensure kvm.Run returns,
// making the vcpu fds available for GetRegs/GetSregs.
// The caller is expected to exit after Save returns.
func (m *Machine) Save(path string) error {
	// Kick all vCPUs out of kvm.Run and stop them so GetRegs/GetSregs
	// can ioctl the vcpu fds without deadlocking.
	atomic.StoreUint32(&m.stopped, 1)
	for _, r := range m.runs {
		r.ImmediateExit = 1
	}

	state := &VMState{
		Mem:   make([]byte, len(m.mem)),
		Regs:  make([]*kvm.Regs, len(m.vcpuFds)),
		Sregs: make([]*kvm.Sregs, len(m.vcpuFds)),
	}

	copy(state.Mem, m.mem)

	if m.serial != nil {
		state.SerialIER = m.serial.IER
		state.SerialLCR = m.serial.LCR
	}

	for i, fd := range m.vcpuFds {
		r, err := kvm.GetRegs(fd)
		if err != nil {
			return fmt.Errorf("Save: GetRegs cpu %d: %w", i, err)
		}
		state.Regs[i] = r

		s, err := kvm.GetSregs(fd)
		if err != nil {
			return fmt.Errorf("Save: GetSregs cpu %d: %w", i, err)
		}
		state.Sregs[i] = s
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("Save: create %s: %w", path, err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(state); err != nil {
		return fmt.Errorf("Save: encode: %w", err)
	}

	return nil
}

// Load restores guest RAM and all vCPU register state from path.
func (m *Machine) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Load: open %s: %w", path, err)
	}
	defer f.Close()

	var state VMState
	if err := gob.NewDecoder(f).Decode(&state); err != nil {
		return fmt.Errorf("Load: decode: %w", err)
	}

	if len(state.Mem) != len(m.mem) {
		return fmt.Errorf("Load: mem size mismatch: state %d vs machine %d", len(state.Mem), len(m.mem))
	}

	copy(m.mem, state.Mem)

	if len(state.Regs) != len(m.vcpuFds) {
		return fmt.Errorf("Load: cpu count mismatch: state %d vs machine %d", len(state.Regs), len(m.vcpuFds))
	}

	for i, fd := range m.vcpuFds {
		if err := kvm.SetRegs(fd, state.Regs[i]); err != nil {
			return fmt.Errorf("Load: SetRegs cpu %d: %w", i, err)
		}
		if err := kvm.SetSregs(fd, state.Sregs[i]); err != nil {
			return fmt.Errorf("Load: SetSregs cpu %d: %w", i, err)
		}
	}

	// Stash serial state for SetupDevices to apply after serial is created.
	m.pendingSerialIER = state.SerialIER
	m.pendingSerialLCR = state.SerialLCR

	return nil
}
