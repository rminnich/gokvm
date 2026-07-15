package vmm

import (
	"github.com/bobuhiro11/gokvm/machine"
)

// Config defines the configuration of the
// virtual machine, as determined by flags.
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
}

type VMM struct {
	*machine.Machine
	Config
}

func New(c Config) *VMM {
	return &VMM{
		Machine: nil,
		Config:  c,
	}
}
