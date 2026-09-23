//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <stdlib.h>
#include "qd_linux.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	appName  = "qd"
	paneFile = "/run/qd-client/ui.json"
)

const (
	itemConnect    = 1
	itemDisconnect = 2
	itemQuit       = 4
)

type pane struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

var (
	client   = &http.Client{Timeout: 3 * time.Second}
	imported atomic.Bool
)

func main() {
	browser := flag.Bool("browser", false, "open the page in the default browser instead of the app window")
	tray := flag.Bool("tray", false, "start in the tray, as the session autostart does")
	flag.Parse()

	if *browser {
		p, err := readPane()
		if err != nil {
			title, text := explain(err)
			fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
			os.Exit(1)
		}
		if err := exec.Command("xdg-open", pageURL(p)).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "xdg-open: %v\n", err)
			os.Exit(1)
		}
		return
	}

	C.qd_init()
	go watch()
	go arrive(*tray)
	atLogin := C.int(0)
	if *tray {
		atLogin = 1
	}
	os.Exit(int(C.qd_start(atLogin)))
}

func readPane() (pane, error) {
	var p pane
	raw, err := os.ReadFile(paneFile)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if p.URL == "" || p.Token == "" {
		return p, errors.New("the service left an empty handoff file")
	}
	return p, nil
}

func pageURL(p pane) string {
	return p.URL + "?t=" + url.QueryEscape(p.Token)
}

func explain(err error) (string, string) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "The qd service is not running",
			"Start it with: sudo systemctl enable --now qd-client"
	case errors.Is(err, fs.ErrPermission):
		return "This account may not open the qd window",
			"Add it to the qd-client group and log in again: sudo usermod -aG qd-client " + os.Getenv("USER")
	}
	return "The qd window could not start", err.Error()
}

//export qdOpen
func qdOpen() {
	p, err := readPane()
	if err != nil {
		title, text := explain(err)
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
		t, x := C.CString(title), C.CString(text)
		defer C.free(unsafe.Pointer(t))
		defer C.free(unsafe.Pointer(x))
		C.qd_complain(t, x)
		return
	}
	uri := C.CString(pageURL(p))
	defer C.free(unsafe.Pointer(uri))
	C.qd_show(uri)
}

//export qdTrayAction
func qdTrayAction(id C.int) {
	go func() {
		p, err := readPane()
		switch id {
		case itemConnect:
			if err != nil || !imported.Load() {
				C.qd_post_show()
				return
			}
			err = call(p, http.MethodPost, "/client/api/connect", nil)
		case itemDisconnect:
			if err == nil {
				err = call(p, http.MethodPost, "/client/api/disconnect", nil)
			}
		case itemQuit:
			if err == nil {
				call(p, http.MethodPost, "/client/api/disconnect", nil)
			}
			C.qd_post_quit()
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "tray: %v\n", err)
		}
	}()
}

func call(p pane, method, path string, out any) error {
	u, err := url.Parse(p.URL)
	if err != nil {
		return err
	}
	u.Path, u.RawQuery = path, ""
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-QD-Token", p.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	var env struct {
		Success bool            `json:"success"`
		Msg     string          `json:"msg"`
		Obj     json.RawMessage `json:"obj"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	if !env.Success {
		return errors.New(env.Msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Obj, out)
}

type state struct {
	Imported  bool `json:"imported"`
	Connected bool `json:"connected"`
	Failed    bool `json:"failed"`
	Node      *struct {
		Name string `json:"name"`
	} `json:"node"`
}

func watch() {
	last := ""
	var seen pane
	for ; ; time.Sleep(time.Second) {
		var st state
		p, err := readPane()
		if err == nil {
			if seen.URL != "" && p != seen {
				uri := C.CString(pageURL(p))
				C.qd_post_load(uri)
				C.free(unsafe.Pointer(uri))
			}
			seen = p
			err = call(p, http.MethodGet, "/client/api/state", &st)
		}
		imported.Store(err == nil && st.Imported)

		status, tip := 0, appName+" — отключён"
		switch {
		case err == nil && st.Connected:
			status, tip = 1, appName+" — подключён"
			if st.Node != nil && st.Node.Name != "" {
				tip = appName + " — " + st.Node.Name
			}
		case err == nil && st.Failed:
			status, tip = 2, appName+" — не удалось подключиться"
		}

		key := fmt.Sprint(status, tip, st.Connected)
		if key == last {
			continue
		}
		last = key
		t := C.CString(tip)
		connected := C.int(0)
		if st.Connected {
			connected = 1
		}
		C.qd_post_state(C.int(status), t, connected)
		C.free(unsafe.Pointer(t))
	}
}

func arrive(atLogin bool) {
	var p pane
	var err error
	for i := 0; ; i++ {
		if p, err = readPane(); err == nil || !atLogin || i == 15 {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		if atLogin {
			C.qd_post_quit()
			return
		}
		C.qd_post_show()
		C.qd_post_release()
		return
	}

	for i := 0; i < 30 && C.qd_tray_live() == 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}

	var s struct {
		AutostartBehaviour string `json:"autostartBehaviour"`
		ManualBehaviour    string `json:"manualBehaviour"`
	}
	behaviour := "open"
	if call(p, http.MethodGet, "/client/api/settings", &s) == nil {
		behaviour = s.ManualBehaviour
		if atLogin {
			behaviour = s.AutostartBehaviour
		}
	}
	if C.qd_tray_live() == 0 || behaviour == "open" || behaviour == "openConnect" {
		C.qd_post_show()
	}
	C.qd_post_release()

	if behaviour == "connect" || behaviour == "openConnect" {
		var st state
		if call(p, http.MethodGet, "/client/api/state", &st) == nil && st.Imported && !st.Connected {
			if err := call(p, http.MethodPost, "/client/api/connect", nil); err != nil {
				fmt.Fprintf(os.Stderr, "connect: %v\n", err)
			}
		}
	}
}
