//go:build windows || linux

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jaywehosl/quic-diver/internal/localapi"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

type runOptions struct {
	StatePath string
	UIHost    string
	UIPort    int
	MTU       int
	Readers   int
	Connect   bool
	Key       string
	Autostart bool
	Duration  time.Duration

	DNS bool

	key *qdcrypt.Key
}

var (
	inBrowser bool
	paneDev   string
)

func main() {
	var opts runOptions

	flag.StringVar(&opts.StatePath, "state", defaultStatePath(), "where the client keeps its subscription and settings")
	flag.StringVar(&opts.UIHost, "ui", "127.0.0.1", "address the local page is served on")
	flag.IntVar(&opts.MTU, "mtu", 1500, "mtu of the local stack; packets are injected into the interface, not the PPPoE path")
	flag.IntVar(&opts.Readers, "readers", 1, "capture threads: 1 keeps packet order, more only helps downloads")
	flag.BoolVar(&opts.Connect, "connect", false, "bring the tunnel up at start instead of waiting for the page")
	flag.BoolVar(&opts.Autostart, "autostart", false, "started by the system, so follow the autostart behaviour setting")
	flag.StringVar(&opts.Key, "key", "", "network key, 64 hex chars, empty leaves the transport in the clear")
	flag.DurationVar(&opts.Duration, "duration", 0, "stop after this long, 0 = until interrupt")

	flag.BoolVar(&opts.DNS, "dns", true, "answer names on the tun adapter through the node's resolver")
	flag.BoolVar(&inBrowser, "browser", false, "open the page in the default browser instead of the app window")
	flag.IntVar(&opts.UIPort, "ui-port", localapi.DefaultPort, "port the local page listens on, 0 takes any free one")
	flag.StringVar(&paneDev, "dev", "", "point the window at a vite dev server instead of the built page")
	flag.Parse()

	if err := runClient(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
