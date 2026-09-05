//go:build !linux && !android

package tun

import (
	"context"
	"errors"

	"github.com/jaywehosl/quic-diver/internal/qcli/packet"
)

var errNoTun = errors.New("tun: not available on this platform")

type Source struct{}

func Open(fd int, mtu int) (*Source, error) { return nil, errNoTun }

func (s *Source) Recv(ctx context.Context) ([]packet.Packet, error) { return nil, errNoTun }
func (s *Source) Send(pkts []packet.Packet) error                   { return errNoTun }
func (s *Source) MTU() int                                          { return 0 }
func (s *Source) Close() error                                      { return nil }

var _ packet.Source = (*Source)(nil)
