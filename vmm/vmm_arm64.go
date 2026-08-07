package vmm

import (
	"fmt"
	"runtime"
)

// arm64Runner is a stub Runner for arm64 where KVM is not yet supported.
type arm64Runner struct{}

func (s *arm64Runner) Init() error  { return fmt.Errorf("not supported on arm64") }
func (s *arm64Runner) Setup() error { return fmt.Errorf("not supported on arm64") }
func (s *arm64Runner) Boot() error  { return fmt.Errorf("not supported on arm64") }
func (s *arm64Runner) Close() error { return nil }
func (s *arm64Runner) Info() VMInfo { return VMInfo{Arch: runtime.GOARCH} }

// New returns a *VMM backed by a stub that returns errors on all operations.
func New(c Config) *VMM {
	return &VMM{Runner: &arm64Runner{}, Config: c}
}
