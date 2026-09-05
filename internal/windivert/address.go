//go:build windows

package windivert

import "unsafe"

const addressSize = 80

type Address struct {
	Timestamp int64
	flags     uint32
	reserved  uint32
	data      [64]byte
}

const _ = addressSize - unsafe.Sizeof(Address{})
const _ = unsafe.Sizeof(Address{}) - addressSize

type NetworkData struct {
	IfIdx    uint32
	SubIfIdx uint32
}

func (a *Address) Network() *NetworkData {
	return (*NetworkData)(unsafe.Pointer(&a.data[0]))
}

func (a *Address) Layer() Layer { return Layer(a.flags & 0xFF) }

func (a *Address) SetLayer(l Layer) {
	a.flags = (a.flags &^ 0xFF) | (uint32(l) & 0xFF)
}

func (a *Address) Event() uint8 { return uint8((a.flags >> 8) & 0xFF) }

const (
	bitSniffed = 16
	bitOutbound = 17
	bitLoopback = 18
	bitImpostor = 19
	bitIPv6     = 20
	bitIPChecksum  = 21
	bitTCPChecksum = 22
	bitUDPChecksum = 23
)

func (a *Address) bit(n uint) bool { return a.flags&(1<<n) != 0 }

func (a *Address) setBit(n uint, v bool) {
	if v {
		a.flags |= 1 << n
		return
	}
	a.flags &^= 1 << n
}

func (a *Address) Sniffed() bool  { return a.bit(bitSniffed) }
func (a *Address) Outbound() bool { return a.bit(bitOutbound) }

func (a *Address) IPChecksumValid() bool  { return a.bit(bitIPChecksum) }
func (a *Address) TCPChecksumValid() bool { return a.bit(bitTCPChecksum) }
func (a *Address) UDPChecksumValid() bool { return a.bit(bitUDPChecksum) }
func (a *Address) Loopback() bool { return a.bit(bitLoopback) }
func (a *Address) Impostor() bool { return a.bit(bitImpostor) }
func (a *Address) IPv6() bool     { return a.bit(bitIPv6) }

func (a *Address) SetOutbound(v bool)    { a.setBit(bitOutbound, v) }
func (a *Address) SetLoopback(v bool)    { a.setBit(bitLoopback, v) }
func (a *Address) SetImpostor(v bool)    { a.setBit(bitImpostor, v) }
func (a *Address) SetIPv6(v bool)        { a.setBit(bitIPv6, v) }
func (a *Address) SetIPChecksum(v bool)  { a.setBit(bitIPChecksum, v) }
func (a *Address) SetTCPChecksum(v bool) { a.setBit(bitTCPChecksum, v) }
func (a *Address) SetUDPChecksum(v bool) { a.setBit(bitUDPChecksum, v) }

func (a *Address) Reset() {
	a.Timestamp = 0
	a.flags = 0
	a.reserved = 0
	for i := range a.data {
		a.data[i] = 0
	}
}
