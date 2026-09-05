package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
)

func subscribe(statePath, raw string) {
	link, err := clientstate.ParseLink(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	db, err := clientstate.Open(statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "state: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	nodes := make([]clientstate.Node, 0, len(link.Endpoints))
	for i, e := range link.Endpoints {
		nodes = append(nodes, clientstate.Node{
			ID: i + 1, Name: e.Address, Role: "ingress",
			Address: e.Address, Port: e.Port, LatencyMs: -1,
		})
	}

	sub := clientstate.Subscription{
		URI: link.String(), Key: link.Key, Label: link.Label,
		Tag: link.Label, CreatedAt: now, LastRefresh: now,
	}
	if err := db.SaveSubscription(sub); err != nil {
		fmt.Fprintf(os.Stderr, "subscription: %v\n", err)
		os.Exit(1)
	}
	if err := db.ReplaceNodes(nodes); err != nil {
		fmt.Fprintf(os.Stderr, "nodes: %v\n", err)
		os.Exit(1)
	}

	if link.NetworkKey != "" {
		settings, err := db.Settings()
		if err != nil {
			fmt.Fprintf(os.Stderr, "settings: %v\n", err)
			os.Exit(1)
		}
		settings.NetworkKey = link.NetworkKey
		if err := db.SaveSettings(settings); err != nil {
			fmt.Fprintf(os.Stderr, "settings: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("subscribed as %q, session %d, %d entrypoint(s)\n",
		link.Key, clientstate.SessionID(link.Key), len(nodes))
}
