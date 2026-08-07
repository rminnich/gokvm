package machine

// Save holds the machine-level state needed to resume a VM.
// It is shared across all VMMs running on this machine — memory
// is captured once here rather than once per VMM.
type Save struct {
	// Mem is a snapshot of guest RAM.
	Mem []byte
}

// NewSave captures the current guest RAM into a Save.
func (m *Machine) NewSave() *Save {
	snap := make([]byte, len(m.mem))
	copy(snap, m.mem)
	return &Save{Mem: snap}
}
