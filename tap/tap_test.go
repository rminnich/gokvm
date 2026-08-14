package tap_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/bobuhiro11/gokvm/tap"
)

func TestNew(t *testing.T) { // nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skipf("skipping; must be root")
	}
	tap, err := tap.New("test_tap")
	if err != nil {
		t.Fatal(err)
	}

	err = tap.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWrite(t *testing.T) { // nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skipf("skipping; must be root")
	}
	tap, err := tap.New("test_write")
	if err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_write", "up").Run(); err != nil {
		t.Fatal(err)
	}

	// With IFF_VNET_HDR the tun expects a 10-byte virtio_net_hdr
	// followed by a full ethernet frame (>= ETH_HLEN 14 bytes);
	// anything shorter is rejected with EINVAL.
	frame := make([]byte, 10+14)

	if _, err := tap.Write(frame); err != nil {
		t.Fatal(err)
	}

	if err := tap.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRead(t *testing.T) { // nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skipf("skipping; must be root")
	}
	tap, err := tap.New("test_read")
	if err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_read", "up").Run(); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 20)
	if _, err := tap.Read(buf); err != nil &&
		!errors.Is(err, syscall.EAGAIN) {
		t.Fatal(err)
	}

	if err := tap.Close(); err != nil {
		t.Fatal(err)
	}
}
