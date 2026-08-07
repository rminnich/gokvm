package vmm

import (
	"fmt"
	"runtime"
)

// riscv64Runner is a stub Runner for riscv64 where KVM is not yet supported.
type riscv64Runner struct{}

func (s *riscv64Runner) Init() error  { return fmt.Errorf("not supported on riscv64") }
func (s *riscv64Runner) Setup() error { return fmt.Errorf("not supported on riscv64") }
func (s *riscv64Runner) Boot() error  { return fmt.Errorf("not supported on riscv64") }
func (s *riscv64Runner) Close() error { return nil }
func (s *riscv64Runner) Info() *Save  { return &Save{Arch: runtime.GOARCH} }

// New returns a *VMM backed by a stub that returns errors on all operations.
func New(c Config) *VMM {
	return &VMM{Runner: &riscv64Runner{}, Config: c}
}
