package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
	"golang.org/x/sys/unix"
)

var (
	ErrIONotPermit = errors.New("IO is not permitted for virtio device")
	ErrNoTxPacket  = errors.New("no packet for tx")
	ErrNoRxPacket  = errors.New("no packet for rx")
	ErrVQNotInit   = errors.New("vq not initialized")
	ErrNoRxBuf     = errors.New("no buffer found for rx")
)

const (
	NetIOPortStart = 0x6200
	NetIOPortSize  = 0x100

	// VIRTIO_RING_F_EVENT_IDX: suppress notifications
	// using used_event/avail_event indices.
	// refs https://docs.oasis-open.org/virtio/virtio/v1.1/cs01/virtio-v1.1-cs01.html#x1-280005
	virtioRingFEventIdx = 1 << 29

	// virtio_net feature bits for GSO/checksum offload, per
	// include/uapi/linux/virtio_net.h: MAC=5, GSO=6, GUEST_TSO4=7,
	// GUEST_TSO6=8, GUEST_ECN=9, HOST_TSO4=11, HOST_TSO6=12,
	// HOST_ECN=13. The tun (TUNSETOFFLOAD) does the segmentation,
	// so the device advertises both host and guest offload.
	virtioNetFCSUM      = 1 << 0
	virtioNetFGuestCSUM = 1 << 1
	virtioNetFGuestTSO4 = 1 << 7
	virtioNetFGuestTSO6 = 1 << 8
	virtioNetFGuestECN  = 1 << 9
	virtioNetFHostTSO4  = 1 << 11
	virtioNetFHostTSO6  = 1 << 12
	virtioNetFHostECN   = 1 << 13
)

// virtioNetGSO is the feature set advertised when the tap
// negotiated TUNSETOFFLOAD and carries a 10-byte vnet hdr.
const virtioNetGSO = virtioNetFCSUM | virtioNetFGuestCSUM |
	virtioNetFGuestTSO4 | virtioNetFGuestTSO6 | virtioNetFGuestECN |
	virtioNetFHostTSO4 | virtioNetFHostTSO6 | virtioNetFHostECN

// vringNeedEvent returns true when the ring index advancing
// from old to new requires a notification, given the peer's
// last observed index event.
//
// refs https://github.com/torvalds/linux/blob/master/drivers/virtio/virtio_ring.c
func vringNeedEvent(event, new, old uint16) bool {
	return uint16(new-event-1) < uint16(new-old)
}

type netHdr struct {
	commonHeader commonHeader
	_            netHeader
}

type Net struct {
	Hdr netHdr

	VirtQueue    [2]*VirtQueue
	Mem          []byte
	LastAvailIdx [2]uint16

	tap io.ReadWriter

	rxBuf []byte // reused RX buffer: 10-byte virtio_net_hdr + packet
	txBuf []byte // reused TX accumulation buffer

	useEventIdx bool
	vnetHdr     bool // tap carries a 10-byte vnet hdr
	offload     bool // tap performs GSO/checksum offload

	txIovs [][]byte // reused TX iovecs pointing into guest memory
	rxIovs [][]byte // reused RX iovecs pointing into guest memory

	txKick    chan interface{}
	done      chan struct{}
	closeOnce sync.Once

	irq         uint8
	IRQInjector IRQInjector
}

func (h netHdr) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type netHeader struct {
	_ [6]uint8 // mac
	_ uint16   // netStatus
	_ uint16   // maxVirtQueuePairs
}

func (v *Net) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:    0x1000,
		VendorID:    0x1AF4,
		HeaderType:  0,
		SubsystemID: 1, // Network Card
		Command:     1, // Enable IO port
		BAR: [6]uint32{
			NetIOPortStart | 0x1,
		},
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		InterruptPin: 1,
		// https://www.webopedia.com/reference/irqnumbers/
		InterruptLine: v.irq,
	}
}

func (v *Net) Read(port uint64, bytes []byte) error {
	offset := int(port - NetIOPortStart)

	if int(v.Hdr.commonHeader.queueSEL) >= len(v.VirtQueue) {
		v.Hdr.commonHeader.queueNUM = 0
	} else {
		v.Hdr.commonHeader.queueNUM = QueueSize
	}

	b, err := v.Hdr.Bytes()
	if err != nil {
		return err
	}

	l := len(bytes)
	copy(bytes[:l], b[offset:offset+l])

	// ISR is at offset 19 in the virtio common header.
	// Per the virtio spec, reading ISR clears it.
	if offset <= 19 && offset+l > 19 {
		v.Hdr.commonHeader.isr = 0
	}

	return nil
}

func (v *Net) RxThreadEntry() {
	log.Println("virtio-net: RxThreadEntry started")

	// Wait for incoming packets by polling the tap fd rather
	// than SIGIO: at line rate SIGIO delivery is one signal per
	// packet, which dominates host CPU. poll() has the same
	// blocking semantics with none of the signal machinery.
	fd := int32(-1)
	if f, ok := v.tap.(interface{ FD() int }); ok {
		fd = int32(f.FD())
	}

	var pfds [1]unix.PollFd

	for {
		select {
		case <-v.done:
			log.Println("virtio-net: RxThreadEntry " +
				"received done signal")

			return
		default:
		}

		if fd >= 0 {
			pfds[0] = unix.PollFd{Fd: fd, Events: unix.POLLIN}
			// 100ms timeout so v.done is honored promptly.
			if _, err := unix.Poll(pfds[:], 100); err != nil ||
				pfds[0].Revents&unix.POLLIN == 0 {
				continue
			}
		} else {
			// No pollable fd: fall back to a periodic drain.
			time.Sleep(time.Millisecond)
		}

		for v.RxDrain() == nil {
		}
	}
}

// RxDrain delivers as many packets as are available and
// injects a single IRQ for the whole batch.
func (v *Net) RxDrain() error {
	const sel = 0

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
	}

	usedRing := &v.VirtQueue[sel].UsedRing
	old := LoadU16(&usedRing.Idx)

	injected := false

	for v.Rx() == nil {
		injected = true
	}

	if !injected {
		return ErrNoRxPacket
	}

	if v.useEventIdx {
		// tell the guest we have consumed the available
		// buffers so it keeps kicking when posting more
		v.VirtQueue[sel].UsedRing.availEvent = LoadU16(&v.VirtQueue[sel].AvailRing.Idx)

		// the guest suppresses IRQs by setting used_event;
		// skip the IRQ if it is already processing.
		event := LoadU16(&v.VirtQueue[sel].AvailRing.UsedEvent)
		new := LoadU16(&usedRing.Idx)

		if !vringNeedEvent(event, new, old) {
			return nil
		}
	}

	return v.IRQInjector.InjectVirtioNetIRQ()
}

func (v *Net) Rx() error {
	sel := 0

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == LoadU16(&availRing.Idx) {
		return ErrNoRxBuf
	}

	uidx := LoadU16(&usedRing.Idx)

	// Each avail entry is the head of a descriptor chain built
	// by the driver. With GSO features negotiated the driver
	// posts big receive buffers as chains of MAX_SKB_FRAGS+2
	// descriptors, so walk the chain via the Next pointers the
	// driver laid out rather than consuming one avail entry per
	// descriptor. Consume exactly one avail entry and emit one
	// used entry per packet.
	headDescID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

	// readv fast path: scatter the packet directly into the
	// guest's descriptor chain with one syscall, no host-side
	// rxBuf copy. With vnet hdr the tun presents the 10-byte
	// vnet hdr verbatim, exactly where the guest expects it.
	if v.vnetHdr {
		if rv, ok := v.tap.(interface {
			Readv([][]byte) (int, error)
		}); ok {
			v.rxIovs = v.rxIovs[:0]

			descID := headDescID
			for {
				desc := &v.VirtQueue[sel].DescTable[descID]

				v.rxIovs = append(v.rxIovs,
					v.Mem[desc.Addr:desc.Addr+uint64(desc.Len)])

				if desc.Flags&0x1 != 0 {
					descID = desc.Next
				} else {
					break
				}
			}

			n, err := rv.Readv(v.rxIovs)
			if err != nil {
				return ErrNoRxPacket
			}

			usedRing.Ring[uidx%QueueSize].Idx = uint32(headDescID)
			usedRing.Ring[uidx%QueueSize].Len = uint32(n)

			v.LastAvailIdx[sel]++
			StoreAddU16(&usedRing.Idx, 1)

			v.Hdr.commonHeader.isr = 0x1

			return nil
		}
	}

	// fallback: read the packet into rxBuf, then copy it into
	// the chain. Used by taps without readv (unit tests).
	if v.rxBuf == nil {
		v.rxBuf = make([]byte, 10+65536)
	}

	// read packet from tap device. With vnet hdr the tun
	// presents a 10-byte vnet hdr (same layout as struct
	// virtio_net_hdr) that we forward verbatim so the guest
	// can reassemble GSO superpackets.
	var packet []byte

	if v.vnetHdr {
		n, err := v.tap.Read(v.rxBuf[:])
		if err != nil {
			return ErrNoRxPacket
		}

		packet = v.rxBuf[:n]
	} else {
		n, err := v.tap.Read(v.rxBuf[10:])
		if err != nil {
			return ErrNoRxPacket
		}

		packet = v.rxBuf[:10+n]

		// struct virtio_net_hdr: all zero. No offload is
		// performed, and VIRTIO_NET_F_MRG_RXBUF is not
		// negotiated, so num_buffers is not present.
		clear(packet[:10])
	}

	usedRing.Ring[uidx%QueueSize].Idx = uint32(headDescID)
	usedRing.Ring[uidx%QueueSize].Len = 0

	descID := headDescID
	for len(packet) > 0 {
		desc := &v.VirtQueue[sel].DescTable[descID]
		l := uint32(len(packet))

		if l > desc.Len {
			l = desc.Len
		}

		copy(v.Mem[desc.Addr:desc.Addr+uint64(l)], packet[:l])

		packet = packet[l:]

		usedRing.Ring[uidx%QueueSize].Len += l

		if len(packet) == 0 {
			break
		}

		if desc.Flags&0x1 == 0 {
			// Driver chain ended but packet does not fit:
			// drop the remainder.
			break
		}

		descID = desc.Next
	}

	v.LastAvailIdx[sel]++

	StoreAddU16(&usedRing.Idx, 1)

	v.Hdr.commonHeader.isr = 0x1

	return nil
}

func (v *Net) TxThreadEntry() {
	log.Println("virtio-net: TxThreadEntry started")

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-v.done:
			log.Println("virtio-net: TxThreadEntry " +
				"received done signal")

			return
		case <-v.txKick:
			for v.Tx() == nil {
			}
		case <-ticker.C:
			for v.Tx() == nil {
			}

			if v.Hdr.commonHeader.isr != 0 {
				_ = v.IRQInjector.InjectVirtioNetIRQ()
			}
		}
	}
}

func (v *Net) Tx() error {
	const sel = 1

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == LoadU16(&availRing.Idx) {
		return ErrNoTxPacket
	}

	old := LoadU16(&usedRing.Idx)

	for v.LastAvailIdx[sel] != LoadU16(&availRing.Idx) {
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		uidx := LoadU16(&usedRing.Idx)
		usedRing.Ring[uidx%QueueSize].Idx = uint32(descID)
		usedRing.Ring[uidx%QueueSize].Len = 0

		// Build a writev iovec list pointing directly into
		// guest memory: one iovec per descriptor, so the tap
		// gathers the packet without a contiguous copy.
		v.txIovs = v.txIovs[:0]
		for {
			desc := v.VirtQueue[sel].DescTable[descID]

			v.txIovs = append(v.txIovs,
				v.Mem[desc.Addr:desc.Addr+uint64(desc.Len)])

			usedRing.Ring[uidx%QueueSize].Len += desc.Len

			if desc.Flags&0x1 != 0 {
				descID = desc.Next
			} else {
				break
			}
		}

		// With vnet hdr the tun consumes the 10-byte vnet hdr
		// (same layout as struct virtio_net_hdr) and performs
		// the segmentation, so write the whole buffer.
		// Otherwise skip the hdr; the tun expects raw frames.
		if err := v.tapWrite(v.txIovs); err != nil {
			return err
		}

		StoreAddU16(&usedRing.Idx, 1)
		v.LastAvailIdx[sel]++
	}
	v.Hdr.commonHeader.isr = 0x1

	if v.useEventIdx {
		v.VirtQueue[sel].UsedRing.availEvent = LoadU16(&v.VirtQueue[sel].AvailRing.Idx)

		event := LoadU16(&v.VirtQueue[sel].AvailRing.UsedEvent)
		new := LoadU16(&usedRing.Idx)

		if !vringNeedEvent(event, new, old) {
			return nil
		}
	}

	return v.IRQInjector.InjectVirtioNetIRQ()
}

// tapWrite delivers a TX packet to the tap. When the tap supports
// writev and carries a vnet hdr, the iovecs already point straight
// into guest memory and are written in one syscall, avoiding a
// contiguous copy. Otherwise the packet is gathered into txBuf.
func (v *Net) tapWrite(iovs [][]byte) error {
	if v.vnetHdr {
		if wv, ok := v.tap.(interface {
			Writev([][]byte) (int, error)
		}); ok {
			if _, err := wv.Writev(iovs); err != nil {
				return err
			}

			return nil
		}
	}

	// fallback for taps without writev: gather into txBuf.
	v.txBuf = v.txBuf[:0]
	for _, b := range iovs {
		off := len(v.txBuf)

		if off+len(b) > cap(v.txBuf) {
			grown := make([]byte, off+len(b), 2*(off+len(b)))
			copy(grown, v.txBuf)
			v.txBuf = grown
		}

		v.txBuf = v.txBuf[:off+len(b)]
		copy(v.txBuf[off:], b)
	}

	buf := v.txBuf
	if !v.vnetHdr {
		// refs https://github.com/torvalds/linux/blob/38f80f42/include/uapi/linux/virtio_net.h#L178-L191
		buf = v.txBuf[10:]
	}

	_, err := v.tap.Write(buf)

	return err
}

func (v *Net) Write(port uint64, bytes []byte) error {
	offset := int(port - NetIOPortStart)

	switch offset {
	case 4:
		// Guest features: the guest accepts a subset of the
		// device features. Track whether event_idx is used.
		features := pci.BytesToNum(bytes)
		v.Hdr.commonHeader.guestFeatures = uint32(features)
		v.useEventIdx = features&virtioRingFEventIdx != 0
	case 8:
		// Queue PFN is aligned to page (4096 bytes)
		sel := v.Hdr.commonHeader.queueSEL
		if int(sel) >= len(v.VirtQueue) {
			break
		}

		physAddr := uint32(pci.BytesToNum(bytes) * 4096)
		v.VirtQueue[sel] = (*VirtQueue)(unsafe.Pointer(&v.Mem[physAddr]))
	case 14:
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		queueIdx := pci.BytesToNum(bytes)
		switch queueIdx {
		case 0:
			// RX queue kick: silently drop.
			// RX is driven by polling the tap fd.
		case 1:
			// TX queue kick: non-blocking send.
			select {
			case v.txKick <- true:
			default:
			}
		default:
			log.Printf(
				"virtio-net: unexpected queue %d",
				queueIdx,
			)
		}
	case 19:
	default:
	}

	return nil
}

func (v *Net) IOPort() uint64 {
	return NetIOPortStart
}

func (v *Net) Size() uint64 {
	return NetIOPortSize
}

func (v *Net) Close() error {
	log.Println("virtio-net: Close called")

	v.closeOnce.Do(func() { close(v.done) })

	if c, ok := v.tap.(io.Closer); ok {
		return c.Close()
	}

	return nil
}

func NewNet(irq uint8, irqInjector IRQInjector, tap io.ReadWriter, mem []byte) *Net {
	hostFeatures := uint32(virtioRingFEventIdx)

	vnetHdr := false
	if f, ok := tap.(interface{ VnetHdr() bool }); ok {
		vnetHdr = f.VnetHdr()
	}

	// Advertise GSO only when the tun actually does offload AND
	// carries a vnet hdr (TUNSETOFFLOAD silently succeeds even
	// without IFF_VNET_HDR, where it would corrupt framing).
	offload := false
	if f, ok := tap.(interface{ Offload() bool }); ok {
		offload = f.Offload() && vnetHdr
	}

	if offload {
		hostFeatures |= virtioNetGSO
	}

	res := &Net{
		Hdr: netHdr{
			commonHeader: commonHeader{
				hostFeatures: hostFeatures,
				queueNUM:     QueueSize,
				isr:          0x0,
			},
		},
		irq:          irq,
		IRQInjector:  irqInjector,
		vnetHdr:      vnetHdr,
		offload:      offload,
		txKick:       make(chan interface{}, QueueSize),
		done:         make(chan struct{}),
		tap:          tap,
		Mem:          mem,
		VirtQueue:    [2]*VirtQueue{},
		LastAvailIdx: [2]uint16{0, 0},
	}

	return res
}
