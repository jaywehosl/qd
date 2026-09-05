//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

type shell struct {
	mu   sync.Mutex
	live webview2.WebView
	up   bool
}

var (
	pane      shell
	inBrowser bool
	paneData  string
	paneDev   string
	paneToken string
)

func (s *shell) show(url string) {
	if url == "" {
		return
	}

	s.mu.Lock()
	if s.up {
		open := s.live
		s.mu.Unlock()
		if open != nil {
			open.Dispatch(func() { front(open) })
		}
		return
	}
	s.up = true
	s.mu.Unlock()

	go s.carry(url)
}

func (s *shell) carry(url string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		s.mu.Lock()
		s.live, s.up = nil, false
		s.mu.Unlock()
	}()

	view := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     true,
		AutoFocus: true,
		DataPath:  paneData,
		WindowOptions: webview2.WindowOptions{
			Title:  appName,
			Width:  1280,
			Height: 860,
			IconId: 1,
			Center: true,
		},
	})
	if view == nil {
		fmt.Printf("window   webview2 runtime is missing, falling back to the browser\n")
		openBrowser(url)
		return
	}
	defer view.Destroy()

	s.mu.Lock()
	s.live = view
	s.mu.Unlock()

	handle := uintptr(view.Window())
	dress(handle, false)
	chrome.take(handle)

	view.Bind("qdWindowTheme", func(dark bool) {
		view.Dispatch(func() { dress(handle, dark) })
	})
	view.Bind("qdTitleBar", func(height float64, holes [][]float64) {
		if height > 0 {
			chrome.setBar(int32(height))
		}
		out := make([]rect, 0, len(holes))
		for _, h := range holes {
			if len(h) < 4 {
				continue
			}
			out = append(out, rect{
				Left: int32(h[0]), Top: int32(h[1]),
				Right: int32(h[0] + h[2]), Bottom: int32(h[1] + h[3]),
			})
		}
		chrome.setHoles(out)
	})
	view.Bind("qdWindowCommand", func(what string) {
		view.Dispatch(func() { command(handle, what) })
	})
	view.Bind("qdWindowGrab", func(edge string) {
		view.Dispatch(func() { grab(handle, edge) })
	})

	if paneDev != "" {
		seed, err := json.Marshal(paneToken)
		if err == nil {
			view.Init(fmt.Sprintf("window.QD_TOKEN=%s;window.X_UI_BASE_PATH='/';"+
				"try{sessionStorage.setItem('qd.token',%s)}catch(e){}", seed, seed))
		}
		url = paneDev
		fmt.Printf("window   dev page %s\n", url)
	}

	view.Navigate(url)
	view.Run()
}

const (
	dwmDarkMode      = 20
	dwmBorderColour  = 34
	dwmCaptionColour = 35
	dwmTextColour    = 36
)

func dress(handle uintptr, dark bool) {
	if handle == 0 {
		return
	}

	caption := uint32(0x00FFFFFF)
	text := uint32(0x00000000)
	mode := uint32(0)
	if dark {
		caption = 0x00000000
		text = 0x00FFFFFF
		mode = 1
	}

	set := func(attr uint32, value uint32) {
		dwmSetAttribute.Call(handle, uintptr(attr),
			uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	}

	set(dwmDarkMode, mode)
	set(dwmCaptionColour, caption)
	set(dwmBorderColour, caption)
	set(dwmTextColour, text)
}

var (
	dwmapi           = windows.NewLazySystemDLL("dwmapi.dll")
	dwmSetAttribute  = dwmapi.NewProc("DwmSetWindowAttribute")
	user32           = windows.NewLazySystemDLL("user32.dll")
	showWindowCall   = user32.NewProc("ShowWindow")
	setForegroundWin = user32.NewProc("SetForegroundWindow")
	isIconicCall     = user32.NewProc("IsIconic")
)

const swRestore = 9

func front(view webview2.WebView) {
	handle := uintptr(view.Window())
	if handle == 0 {
		return
	}
	if iconic, _, _ := isIconicCall.Call(handle); iconic != 0 {
		showWindowCall.Call(handle, swRestore)
	}
	setForegroundWin.Call(handle)
}
