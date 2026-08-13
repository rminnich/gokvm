package ebda_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/ebda"
)

func TestNew(t *testing.T) {
	t.Parallel()

	m, err := ebda.New(4)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// Serialized layout: padding(48) + mpfIntel(16) + MP table header(44)
	// + bus(8) + IOAPIC(8) + 16 I/O interrupt entries(8 each) + nCPUs CPU
	// entries(20 each).
	if want := 252 + 4*20; len(bytes) != want {
		t.Fatalf("Invalid size: %v, want %v", len(bytes), want)
	}
}
