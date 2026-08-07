package vmm

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/pvh"
	"github.com/bobuhiro11/gokvm/term"
	"golang.org/x/sync/errgroup"
)

// amd64VMM implements Runner for the amd64 architecture.
type amd64VMM struct {
	m *machine.Machine
	c Config
}

// New returns a *VMM backed by the amd64 implementation.
func New(c Config) *VMM {
	return &VMM{Runner: &amd64VMM{c: c}, Config: c}
}

func (v *amd64VMM) Init() error {
	m, err := machine.New(v.c.Dev, v.c.NCPUs, v.c.MemSize)
	if err != nil {
		return err
	}

	if len(v.c.TapIfName) > 0 {
		if err := m.AddTapIf(v.c.TapIfName); err != nil {
			return err
		}
	}

	if len(v.c.Disk) > 0 {
		if err := m.AddDisk(v.c.Disk); err != nil {
			return err
		}
	}

	v.m = m

	return nil
}

func (v *amd64VMM) Setup() error {
	if v.c.Resume != "" {
		if err := v.m.Load(v.c.Resume); err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		if err := v.m.SetupDevices(); err != nil {
			return fmt.Errorf("resume SetupDevices: %w", err)
		}
		log.Printf("Resumed from %s", v.c.Resume)
		return nil
	}

	kern, err := os.Open(v.c.Kernel)
	if err != nil {
		return err
	}

	isPVH, err := pvh.CheckPVH(kern)
	if err != nil {
		return err
	}

	var initrd *os.File
	if v.c.Initrd != "" {
		initrd, err = os.Open(v.c.Initrd)
		if err != nil {
			return err
		}
	}

	if isPVH {
		return v.m.LoadPVH(kern, initrd, v.c.Params)
	}

	return v.m.LoadLinux(kern, initrd, v.c.Params)
}

func (v *amd64VMM) Boot() error {
	savePath := v.c.SavePath
	if savePath == "" {
		savePath = "gokvm.state"
	}

	save := func() error {
		log.Printf("Saving state to %s", savePath)
		if err := v.m.Save(savePath); err != nil {
			return err
		}
		log.Printf("State saved to %s", savePath)
		return nil
	}

	// Save is triggered in-band via ^A^Z through the serial console.
	// SIGHUP-based save is deferred to a future implementation.

	trace := v.c.TraceCount > 0
	if err := v.m.SingleStep(trace); err != nil {
		return fmt.Errorf("setting trace to %v:%w", trace, err)
	}

	g := new(errgroup.Group)

	for cpu := 0; cpu < v.c.NCPUs; cpu++ {
		fmt.Printf("Start CPU %d of %d\r\n", cpu, v.c.NCPUs)
		i := cpu
		g.Go(func() error {
			return v.m.VCPU(os.Stderr, i, v.c.TraceCount)
		})
	}

	if !term.IsTerminal() {
		fmt.Fprintln(os.Stderr, "this is not terminal and does not accept input")
		select {}
	}

	restoreMode, err := term.SetRawMode()
	if err != nil {
		return err
	}

	defer restoreMode()

	in := bufio.NewReader(os.Stdin)

	g.Go(func() error {
		err := v.m.GetSerial().Start(*in, restoreMode, v.m.InjectSerialIRQ, save)
		log.Printf("Serial exits: %v", err)
		err = v.Close()
		log.Printf("close returns %v", err)
		return err
	})

	fmt.Printf("Waiting for CPUs to exit\r\n")

	if err := g.Wait(); err != nil {
		log.Print(err)
	}

	fmt.Printf("All cpus done\n\r")

	return nil
}

// Close stops the amd64 VMM by calling machine.Close(), which sets
// ImmediateExit and sends SIGUSR1 to each vCPU thread via tgkill.
func (v *amd64VMM) Close() error {
	if v.m == nil {
		return nil
	}

	return v.m.Close()
}

// Info returns a Save containing guest RAM (once, in Machine) and per-vCPU
// register state captured when the VMM was stopped.
// Returns nil if the VMM has not been initialised.
func (v *amd64VMM) Info() *Save {
	if v.m == nil {
		return nil
	}

	save := &Save{
		Arch:    runtime.GOARCH,
		Machine: v.m.NewSave(),
	}

	for cpu := 0; cpu < v.c.NCPUs; cpu++ {
		save.VCPUs = append(save.VCPUs, VCPUSave{
			CPU:  cpu,
			Regs: v.m.GetAMD64State(), // *machine.AMD64State or nil
		})
	}

	return save
}
