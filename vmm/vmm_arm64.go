package vmm

import "fmt"

// stubRunner is returned on architectures that do not support KVM.
type stubRunner struct{}

func (s *stubRunner) Init() error  { return fmt.Errorf("not supported on this architecture") }
func (s *stubRunner) Setup() error { return fmt.Errorf("not supported on this architecture") }
func (s *stubRunner) Boot() error  { return fmt.Errorf("not supported on this architecture") }

// New returns a *VMM backed by a stub that returns errors on all operations.
func New(c Config) *VMM {
	return &VMM{Runner: &stubRunner{}, Config: c}
}
