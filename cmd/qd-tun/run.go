//go:build windows

package main

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"encoding/hex"
	"github.com/jaywehosl/quic-diver/internal/adblock"

	"github.com/jaywehosl/quic-diver/internal/clientapi"
	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
)

func defaultStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "qd-client.db"
	}
	return filepath.Join(dir, "QuicDiver", "client.db")
}

func runClient(opts runOptions) error {
	redirectOutput(opts.StatePath)
	standFull()
	paneData = filepath.Join(filepath.Dir(opts.StatePath), "webview")

	first, release := true, func() {}
	if paneDev == "" {
		first, release = claimInstance()
	}
	if !first {
		if knock() {
			fmt.Printf("single   already running, brought its page up\n")
			return nil
		}
		fmt.Printf("single   already running, but it did not answer\n")
		return nil
	}
	defer release()

	db, err := clientstate.Open(opts.StatePath)
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	defer db.Close()

	reloadProcessRules(db)
	setFixedRate(settingsFixedRate(db))

	settings, err := db.Settings()
	if err != nil {
		return err
	}
	seen := clientapi.NewVisits(db, adblock.Default(), settings.Adblock)
	defer seen.Close()

	keyText := opts.Key
	if keyText == "" {
		keyText = settings.NetworkKey
	}

	var key *qdcrypt.Key
	mtu := opts.MTU
	if keyText != "" {
		raw, err := hex.DecodeString(keyText)
		if err != nil || len(raw) != qdcrypt.KeySize {
			return fmt.Errorf("key must be %d hex chars", qdcrypt.KeySize*2)
		}
		var k qdcrypt.Key
		copy(k[:], raw)
		key = &k
		opts.key = key
		if settings.NetworkKey != keyText {
			settings.NetworkKey = keyText
			if err := db.SaveSettings(settings); err != nil {
				fmt.Printf("crypto   key not stored: %v\n", err)
			}
		}
		fmt.Printf("crypto   chacha20, mtu %d\n", mtu)
	} else {
		fmt.Printf("crypto   off, no network key\n")
	}

	var admin *adminUI
	var api *clientapi.API

	var tun *tunnel
	tun = newTunnel(tunnelConfig{
		MTU:     mtu,
		Workers: opts.Readers,
		DNS:     opts.DNS,
		OnQuery: seen.Query,
		Token: func() string {
			sub, err := db.Subscription()
			if err != nil {
				return ""
			}
			return sub.Key
		},
		Peers: func() []string {
			out := []string{}
			if nodes, err := db.Nodes(); err == nil {
				for _, n := range nodes {
					out = append(out, n.Address)
				}
			}
			if api != nil {
				out = append(out, api.Peers()...)
			}
			if admin != nil {
				if fleet, _ := admin.handler(); fleet != nil {
					for _, n := range fleet.Nodes() {
						out = append(out, n.Address)
					}
				}
			}
			return out
		},
		Lost: func() {
			time.Sleep(3 * time.Second)
			sub, err := db.Subscription()
			if err != nil || !sub.Imported {
				return
			}
			if err := connectNow(db, tun, sub, opts.key); err != nil {
				fmt.Printf("carry    could not come back: %v\n", err)
			}
		},
		Announce: func(op string) {
			sub, err := db.Subscription()
			if err != nil || !sub.Imported {
				return
			}
			nodes, err := db.Nodes()
			if err != nil {
				return
			}
			if api == nil {
				return
			}
			clientapi.Announce(op, nodes, api.Key(), sub.Key, deviceOf(), nodeTalk)
		},
		Key: key,
	})

	api = clientapi.New(db, winPlatform{tun: tun, db: db}, seen, opts.key)

	admin = newAdminUI(key, db)
	api.OnImport = func() {
		admin.SetKey(api.Key())
		go api.Greet()
	}
	go api.Greet()

	ui, err := startLocalUI(opts.UIHost, opts.UIPort, api.Routes(), admin, func() bool {
		sub, err := db.Subscription()
		return err == nil && sub.Admin
	})
	if err != nil {
		return fmt.Errorf("local page: %w", err)
	}

	ui.SetFeed(admin.live)

	pageURL := ui.URL()
	paneToken = ui.Token()
	if paneDev != "" {
		if u, err := url.Parse(paneDev); err == nil {
			ui.Allow(u.Scheme + "://" + u.Host)
		}
	}
	fmt.Printf("state    %s\n", opts.StatePath)
	fmt.Printf("page     %s\n", pageURL)

	sub, err := db.Subscription()
	if err != nil {
		return err
	}
	if sub.Imported {
		fmt.Printf("key      %s (%s)\n", shorten(sub.Key), sub.Label)
	} else {
		fmt.Printf("key      none yet — open the page and paste a qd:// link\n")
	}

	stop := make(chan struct{})
	go collectSamples(db, tun, stop)
	go api.KeepFresh(stop)
	go api.KeepProbing(stop, 60*time.Second)
	go answerKnocks(func() { openPage(pageURL) }, stop)

	quit := make(chan struct{})
	icon, err := startTray(db, tun, ui, key, quit)
	if err != nil {
		fmt.Printf("tray     %v\n", err)
	} else {
		fmt.Printf("tray     running\n")
		defer icon.Stop()
		go watchTray(icon, db, tun, stop)
	}

	if opts.Connect && sub.Imported {
		if err := connectNow(db, tun, sub, opts.key); err != nil {
			fmt.Printf("connect  %v\n", err)
		}
	}

	behaviour := settings.ManualBehaviour
	if opts.Autostart {
		behaviour = settings.AutostartBehaviour
	}
	if icon == nil || behaviour == "open" {
		openPage(pageURL)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	var deadline <-chan time.Time
	if opts.Duration > 0 {
		t := time.NewTimer(opts.Duration)
		defer t.Stop()
		deadline = t.C
	}

	select {
	case <-sig:
	case <-quit:
	case <-deadline:
	}

	close(stop)
	tun.Release()
	return nil
}

func shorten(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "…"
}
