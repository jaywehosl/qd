//go:build windows && !core

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/jaywehosl/qd/internal/update"
)

type shell struct {
	mu   sync.Mutex
	live webview2.WebView
}

var (
	pane      shell
	paneData  string
	paneToken string
)

const (
	windowFlag  = "-window"
	windowMutex = `Global\qdWindow`
	windowShow  = `Global\qdWindowShow`
	windowShut  = `Global\qdWindowShut`
)

func init() {
	if len(os.Args) > 3 && os.Args[1] == windowFlag {
		hostWindow(os.Args[2:])
		os.Exit(0)
	}
}

func nudge(event string) bool {
	name, err := windows.UTF16PtrFromString(event)
	if err != nil {
		return false
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	return windows.SetEvent(handle) == nil
}

func windowUp() bool {
	name, err := windows.UTF16PtrFromString(windowMutex)
	if err != nil {
		return false
	}
	held, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(held)
	return true
}

func shutWindow() {
	if paneDev != "" || !nudge(windowShut) {
		return
	}
	for i := 0; i < 30 && windowUp(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

func hostWindow(args []string) {
	name, err := windows.UTF16PtrFromString(windowMutex)
	if err != nil {
		return
	}
	held, err := windows.CreateMutex(nil, false, name)
	if held == 0 || err == windows.ERROR_ALREADY_EXISTS {
		nudge(windowShow)
		return
	}
	defer windows.CloseHandle(held)

	paneData = args[1]
	if len(args) > 3 {
		paneDev, paneToken = args[2], args[3]
	}
	show, shut := namedEvent(windowShow, false), namedEvent(windowShut, false)

	go func() {
		for {
			which, err := windows.WaitForMultipleObjects([]windows.Handle{show, shut}, false, windows.INFINITE)
			if err != nil {
				return
			}
			pane.mu.Lock()
			open := pane.live
			pane.mu.Unlock()
			if which != windows.WAIT_OBJECT_0 {
				if open == nil {
					os.Exit(0)
				}
				open.Dispatch(open.Terminate)
				return
			}
			if open != nil {
				open.Dispatch(func() { front(open) })
			}
		}
	}()
	pane.carry(args[0])
}

const (
	guardPage = false
	headless  = false
)

func bindPane(statePath, url, token string) {
	paneData = filepath.Join(filepath.Dir(statePath), "webview")
	paneToken = token
	if !inherited {
		shutWindow()
	}
}

func (s *shell) show(url string) {
	if url == "" || nudge(windowShow) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	args := []string{windowFlag, url, paneData}
	if paneDev != "" {
		args = append(args, paneDev, paneToken)
	}
	host := exec.Command(exe, args...)
	host.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := host.Start(); err != nil {
		fmt.Printf("window   not opened: %v\n", err)
		return
	}
	host.Process.Release()
}

func (s *shell) carry(url string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		s.mu.Lock()
		s.live = nil
		s.mu.Unlock()
	}()

	view := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     paneDev != "",
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
	fit(handle, 1280, 860)
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
	view.Bind("qdOpenURL", func(target string) {
		if strings.HasPrefix(target, "https://github.com/"+update.Repo+"/") {
			openBrowser(target)
		}
	})

	if paneDev != "" {
		seed, err := json.Marshal(paneToken)
		if err == nil {
			view.Init(fmt.Sprintf("window.QD_TOKEN=%s;window.X_UI_BASE_PATH='/';"+
				"try{sessionStorage.setItem('qd.token',%s)}catch(e){}", seed, seed))
		}
		url = paneDev
		fmt.Printf("window   dev page %s, api token %s\n", url, paneToken)
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

var (
	getDpiForWindow = user32.NewProc("GetDpiForWindow")
	monitorFromWin  = user32.NewProc("MonitorFromWindow")
	getMonitorInfo  = user32.NewProc("GetMonitorInfoW")
)

const (
	monitorNearest = 2
	swpNoActivate  = 0x0010
)

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

func fit(handle uintptr, wide, tall int32) {
	if handle == 0 {
		return
	}

	dpi := uintptr(96)
	if got, _, _ := getDpiForWindow.Call(handle); got != 0 {
		dpi = got
	}
	w := wide * int32(dpi) / 96
	h := tall * int32(dpi) / 96

	mon, _, _ := monitorFromWin.Call(handle, monitorNearest)
	if mon == 0 {
		setWindowPos.Call(handle, 0, 0, 0, uintptr(w), uintptr(h),
			uintptr(swpNoMove|swpNoZOrder|swpNoActivate))
		return
	}

	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := getMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&info))); ok == 0 {
		setWindowPos.Call(handle, 0, 0, 0, uintptr(w), uintptr(h),
			uintptr(swpNoMove|swpNoZOrder|swpNoActivate))
		return
	}

	room := info.Work
	if wide := (room.Right - room.Left) * 92 / 100; w > wide {
		w = wide
	}
	if tall := (room.Bottom - room.Top) * 92 / 100; h > tall {
		h = tall
	}

	x := room.Left + (room.Right-room.Left-w)/2
	y := room.Top + (room.Bottom-room.Top-h)/2
	setWindowPos.Call(handle, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(swpNoZOrder|swpNoActivate))
}
