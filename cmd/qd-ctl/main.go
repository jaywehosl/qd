package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qwire"
)

func main() {
	addr := flag.String("node", "", "node address, host:port")
	keyHex := flag.String("key", "", "network key, 64 hex chars")
	auth := flag.String("auth", "", "admin token, the uuid from a qd:// link")
	op := flag.String("op", "hello", "operation to ask for")
	body := flag.String("body", "", "json body for the operation")
	_ = flag.Duration("timeout", 4*time.Second, "kept for habit: the wait now lives in the transport")
	statePath := flag.String("state", "", "a client state file to write a subscription into")
	link := flag.String("link", "", "qd:// link to subscribe that state file to")
	flag.Parse()

	if *statePath != "" || *link != "" {
		if *statePath == "" || *link == "" {
			fmt.Fprintln(os.Stderr, "usage: qd-ctl -state <client.db> -link qd://...")
			os.Exit(2)
		}
		subscribe(*statePath, *link)
		return
	}

	if *addr == "" || *keyHex == "" {
		fmt.Fprintln(os.Stderr, "usage: qd-ctl -node host:port -key <64 hex> [-op hello]")
		os.Exit(2)
	}

	raw, err := hex.DecodeString(*keyHex)
	if err != nil || len(raw) != qdcrypt.KeySize {
		fmt.Fprintf(os.Stderr, "key must be %d hex chars\n", qdcrypt.KeySize*2)
		os.Exit(2)
	}
	talk := qwire.New()
	talk.SetToken(*keyHex)

	var payload any
	if *body != "" {
		payload = json.RawMessage(*body)
	}

	started := time.Now()
	answer, err := talk.Raw(*addr, *op, *auth, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%d ms  %s\n", time.Since(started).Milliseconds(), answer)
}
