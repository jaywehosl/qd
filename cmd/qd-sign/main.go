package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/jaywehosl/qd/internal/update"
)

func main() {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("QD_UPDATE_KEY")))
	if err != nil || len(seed) != ed25519.SeedSize {
		fmt.Fprintln(os.Stderr, "QD_UPDATE_KEY must hold the base64 seed of the update key")
		os.Exit(1)
	}
	key := ed25519.NewKeyFromSeed(seed)

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: qd-sign checksums.txt")
		os.Exit(1)
	}
	for _, path := range os.Args[1:] {
		body, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if tag, _ := update.SumOf(body, update.Label); !strings.HasPrefix(tag, "v") {
			fmt.Fprintf(os.Stderr, "%s names no release: add the line \"vX.Y.Z-alpha *%s\"\n", path, update.Label)
			os.Exit(1)
		}
		sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, body)) + "\n")
		if !update.Signed(body, sig) {
			fmt.Fprintln(os.Stderr, "this key is not the one the clients trust")
			os.Exit(1)
		}
		if err := os.WriteFile(path+".sig", sig, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("signed %s\n", path)
	}
}
