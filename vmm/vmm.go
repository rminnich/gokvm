package vmm

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/pvh"
	"github.com/bobuhiro11/gokvm/term"
	"golang.org/x/sync/errgroup"
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
	Resume     string // path to state file for -R resume
	SavePath   string // path to write state on save
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

// Init instantiates a machine.
func (v *VMM) Init() error {
	m, err := machine.New(v.Dev, v.NCPUs, v.MemSize)
	if err != nil {
		return err
	}

	if len(v.TapIfName) > 0 {
		if err := m.AddTapIf(v.TapIfName); err != nil {
			return err
		}
	}

	if len(v.Disk) > 0 {
		if err := m.AddDisk(v.Disk); err != nil {
			return err
		}
	}

	v.Machine = m

	return nil
}

func (v *VMM) Setup() error {
	// Resume path: load state from file, skip kernel setup.
	if v.Resume != "" {
		if err := v.Machine.Load(v.Resume); err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		log.Printf("Resumed from %s", v.Resume)
		return nil
	}

	var initrd *os.File
	// Kernel arg required to load kernel or firmware image
	kern, err := os.Open(v.Kernel)
	if err != nil {
		return err
	}

	isPVH, err := pvh.CheckPVH(kern)
	if err != nil {
		return err
	}

	if v.Initrd != "" {
		initrd, err = os.Open(v.Initrd)
		if err != nil {
			return err
		}
	}

	if isPVH {
		if err := v.Machine.LoadPVH(kern, initrd, v.Params); err != nil {
			return err
		}
	} else {
		if err := v.Machine.LoadLinux(kern, initrd, v.Params); err != nil {
			return err
		}
	}

	return nil
}

func (v *VMM) Boot() error {
	var err error

	savePath := v.SavePath
	if savePath == "" {
		savePath = "gokvm.state"
	}

	save := func() error {
		log.Printf("Saving state to %s", savePath)
		if err := v.Machine.Save(savePath); err != nil {
			return err
		}
		log.Printf("State saved to %s", savePath)
		return nil
	}

	// SIGHUP triggers a save.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP)
	go func() {
		for range sigs {
			if err := save(); err != nil {
				log.Printf("SIGHUP save: %v", err)
			}
		}
	}()

	trace := v.TraceCount > 0
	if err := v.SingleStep(trace); err != nil {
		return fmt.Errorf("setting trace to %v:%w", trace, err)
	}

	g := new(errgroup.Group)

	for cpu := 0; cpu < v.NCPUs; cpu++ {
		fmt.Printf("Start CPU %d of %d\r\n", cpu, v.NCPUs)

		i := cpu

		f := func() error {
			return v.VCPU(os.Stderr, i, v.TraceCount)
		}

		g.Go(f)
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
		err := v.GetSerial().Start(*in, restoreMode, v.InjectSerialIRQ, save)
		log.Printf("Serial exits: %v", err)

		return err
	})

	fmt.Printf("Waiting for CPUs to exit\r\n")

	if err := g.Wait(); err != nil {
		log.Print(err)
	}

	fmt.Printf("All cpus done\n\r")

	return nil
}
