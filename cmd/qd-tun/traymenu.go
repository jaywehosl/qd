//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/localapi"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/tray"
)

func startTray(db *clientstate.DB, tun *tunnel, ui *localapi.Server, key *qdcrypt.Key, quit chan struct{}) (*tray.Icon, error) {
	var once sync.Once

	return tray.Run("QuicDiver", tray.Menu{
		Open: func() { openPage(ui.URL()) },

		Connect: func() {
			sub, err := db.Subscription()
			if err != nil || !sub.Imported {
				openPage(ui.URL())
				return
			}
			if err := connectNow(db, tun, sub, key); err != nil {
				fmt.Printf("connect  %v\n", err)
			}
		},

		Disconnect: func() {
			if err := tun.Stop(); err != nil {
				fmt.Printf("disconnect %v\n", err)
			}
		},

		Quit: func() { once.Do(func() { close(quit) }) },

		Connected: tun.Running,
	})
}

func watchTray(icon *tray.Icon, db *clientstate.DB, tun *tunnel, stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	last := tray.Status(-1)
	lastTip := ""

	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}

		status := tray.StatusOff
		switch {
		case tun.Running():
			status = tray.StatusOn
		case tun.Failed():
			status = tray.StatusError
		}

		tip := trayTip(db, tun, status)
		if status == last && tip == lastTip {
			continue
		}
		last, lastTip = status, tip

		icon.SetTooltip(tip)
		icon.SetStatus(status)
	}
}

func trayTip(db *clientstate.DB, tun *tunnel, status tray.Status) string {
	switch status {
	case tray.StatusOn:
		if nodes, err := db.Nodes(); err == nil {
			for _, n := range nodes {
				if n.Selected {
					return "QuicDiver — " + n.Name
				}
			}
		}
		return "QuicDiver — подключён"
	case tray.StatusError:
		return "QuicDiver — не удалось подключиться"
	default:
		return "QuicDiver — отключён"
	}
}

func openPage(url string) {
	if inBrowser {
		openBrowser(url)
		return
	}
	pane.show(url)
}

func openBrowser(url string) {
	if url == "" {
		return
	}
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		fmt.Printf("open     %v\n", err)
		return
	}
	go cmd.Wait()
}
