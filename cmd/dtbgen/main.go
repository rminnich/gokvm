// Command dtbgen generates a standalone devicetree blob (DTB) for the
// arm64 "virt"-style machine implemented by github.com/bobuhiro11/gokvm/dtb
// and github.com/bobuhiro11/gokvm/machine (machine_linux_arm64.go).
//
// It is a plain, cross-platform Go program (it performs no ioctls or
// syscalls), so it can be run on any development machine to inspect or
// validate the devicetree gokvm will generate at boot time, e.g.:
//
//	go run ./cmd/dtbgen -o virt.dtb -mem 268435456 -cpus 4 \
//	        -bootargs "console=ttyAMA0 root=/dev/vda rw"
//	dtc -I dtb -O dts virt.dtb
package main

import (
	"flag"
	"log"
	"os"

	"github.com/bobuhiro11/gokvm/dtb"
)

func main() {
	var (
		out      = flag.String("o", "virt.dtb", "output file path")
		memSize  = flag.Uint64("mem", 1<<28, "guest memory size in bytes")
		nCpus    = flag.Int("cpus", 1, "number of guest vcpus")
		bootargs = flag.String("bootargs", "console=ttyAMA0", "kernel command line")
	)

	flag.Parse()

	blob := dtb.GenerateVirt(*memSize, *nCpus, *bootargs)

	if err := os.WriteFile(*out, blob, 0o644); err != nil {
		log.Fatal(err)
	}
}
