//go:build windows

package tray

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx    = user32.NewProc("RegisterClassExW")
	procCreateWindowEx     = user32.NewProc("CreateWindowExW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procPostMessage        = user32.NewProc("PostMessageW")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procAppendMenu         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procSetForegroundWin   = user32.NewProc("SetForegroundWindow")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procLoadCursor         = user32.NewProc("LoadCursorW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")

	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")

	procCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap     = gdi32.NewProc("CreateBitmap")
	procDeleteObject     = gdi32.NewProc("DeleteObject")

	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy      = 0x0002
	wmCommand      = 0x0111
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	wmTrayIcon     = 0x0400 + 1
	wmSetStatus    = 0x0400 + 2
	wmCloseTray    = 0x0400 + 3
	wmNull         = 0x0000
	csHRedraw      = 0x0002
	idiApplication = 32512
	idcArrow       = 32512

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfGrayed    = 0x00000001

	tpmLeftAlign   = 0x0000
	tpmRightButton = 0x0002

	idConnect    = 1001
	idDisconnect = 1002
	idQuit       = 1003
)

type Status int

const (
	StatusOff Status = iota
	StatusOn
	StatusError
)

type Menu struct {
	Open       func()
	Connect    func()
	Disconnect func()
	Quit       func()

	Connected func() bool
}

type Icon struct {
	hwnd    uintptr
	menu    Menu
	tooltip string

	mu      sync.Mutex
	icons   map[Status]uintptr
	status  Status
	stopped bool
	done    chan struct{}
}

type point struct{ X, Y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type notifyIconData struct {
	size         uint32
	hWnd         uintptr
	uID          uint32
	uFlags       uint32
	uCallbackMsg uint32
	hIcon        uintptr
	szTip        [128]uint16
	dwState      uint32
	dwStateMask  uint32
	szInfo       [256]uint16
	uVersion     uint32
	szInfoTitle  [64]uint16
	dwInfoFlags  uint32
	guidItem     [16]byte
	hBalloonIcon uintptr
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

func Run(tooltip string, m Menu) (*Icon, error) {
	i := &Icon{
		menu:    m,
		tooltip: tooltip,
		icons:   make(map[Status]uintptr, 3),
		done:    make(chan struct{}),
	}

	ready := make(chan error, 1)
	go i.loop(ready)

	if err := <-ready; err != nil {
		return nil, err
	}
	return i, nil
}

func (i *Icon) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(i.done)

	instance, _, _ := procGetModuleHandle.Call(0)
	className := syscall.StringToUTF16Ptr("QuicDiverTray")
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)

	class := wndClassEx{
		style:     csHRedraw,
		wndProc:   syscall.NewCallback(i.wndProc),
		instance:  instance,
		cursor:    cursor,
		className: className,
	}
	class.size = uint32(unsafe.Sizeof(class))

	if atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		ready <- fmt.Errorf("register tray class: %w", err)
		return
	}

	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("QuicDiver"))),
		0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		ready <- fmt.Errorf("create tray window: %w", err)
		return
	}
	i.hwnd = hwnd

	for _, s := range []Status{StatusOff, StatusOn, StatusError} {
		i.icons[s] = makeIcon(s)
	}

	data := i.notifyData(nifMessage | nifIcon | nifTip)
	if ok, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data))); ok == 0 {
		procDestroyWindow.Call(hwnd)
		ready <- fmt.Errorf("add tray icon: %w", err)
		return
	}

	ready <- nil

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	remove := i.notifyData(0)
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&remove)))
	for _, h := range i.icons {
		if h != 0 {
			procDestroyIcon.Call(h)
		}
	}
}

func (i *Icon) notifyData(flags uint32) notifyIconData {
	i.mu.Lock()
	icon := i.icons[i.status]
	tip := i.tooltip
	i.mu.Unlock()

	d := notifyIconData{
		hWnd:         i.hwnd,
		uID:          1,
		uFlags:       flags,
		uCallbackMsg: wmTrayIcon,
		hIcon:        icon,
	}
	d.size = uint32(unsafe.Sizeof(d))
	copyTip(&d.szTip, tip)
	return d
}

func copyTip(dst *[128]uint16, text string) {
	encoded := syscall.StringToUTF16(text)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst[:], encoded)
}

func (i *Icon) wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTrayIcon:
		switch uint32(lParam) {
		case wmLButtonUp:
			i.call(i.menu.Open)
		case wmRButtonUp:
			i.showMenu()
		}
		return 0

	case wmCommand:
		switch uint16(wParam) {
		case idConnect:
			i.call(i.menu.Connect)
		case idDisconnect:
			i.call(i.menu.Disconnect)
		case idQuit:
			i.call(i.menu.Quit)
		}
		return 0

	case wmSetStatus:
		i.mu.Lock()
		i.status = Status(wParam)
		i.mu.Unlock()
		d := i.notifyData(nifIcon | nifTip)
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&d)))
		return 0

	case wmCloseTray:
		procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func (i *Icon) call(fn func()) {
	if fn == nil {
		return
	}
	go fn()
}

func (i *Icon) showMenu() {
	hmenu, _, _ := procCreatePopupMenu.Call()
	if hmenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hmenu)

	connected := false
	if i.menu.Connected != nil {
		connected = i.menu.Connected()
	}

	connectFlags := uintptr(mfString)
	disconnectFlags := uintptr(mfString)
	if connected {
		connectFlags |= mfGrayed
	} else {
		disconnectFlags |= mfGrayed
	}

	appendItem(hmenu, connectFlags, idConnect, "Подключиться")
	appendItem(hmenu, disconnectFlags, idDisconnect, "Отключиться")
	procAppendMenu.Call(hmenu, mfSeparator, 0, 0)
	appendItem(hmenu, mfString, idQuit, "Выход")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWin.Call(i.hwnd)
	procTrackPopupMenu.Call(hmenu, tpmLeftAlign|tpmRightButton,
		uintptr(pt.X), uintptr(pt.Y), 0, i.hwnd, 0)
	procPostMessage.Call(i.hwnd, wmNull, 0, 0)
}

func appendItem(hmenu, flags uintptr, id uint32, text string) {
	procAppendMenu.Call(hmenu, flags, uintptr(id),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))))
}

func (i *Icon) SetStatus(s Status) {
	i.mu.Lock()
	stopped := i.stopped
	i.mu.Unlock()
	if stopped || i.hwnd == 0 {
		return
	}
	procPostMessage.Call(i.hwnd, wmSetStatus, uintptr(s), 0)
}

func (i *Icon) SetTooltip(text string) {
	i.mu.Lock()
	i.tooltip = text
	status := i.status
	stopped := i.stopped
	i.mu.Unlock()
	if stopped || i.hwnd == 0 {
		return
	}
	procPostMessage.Call(i.hwnd, wmSetStatus, uintptr(status), 0)
}

func (i *Icon) Stop() {
	i.mu.Lock()
	if i.stopped {
		i.mu.Unlock()
		return
	}
	i.stopped = true
	i.mu.Unlock()

	if i.hwnd != 0 {
		procPostMessage.Call(i.hwnd, wmCloseTray, 0, 0)
	}
	<-i.done
}

const iconSize = 32

func makeIcon(s Status) uintptr {
	var r, g, b byte
	switch s {
	case StatusOn:
		r, g, b = 0x34, 0xa8, 0x53
	case StatusError:
		r, g, b = 0xea, 0x43, 0x35
	default:
		r, g, b = 0x8a, 0x8a, 0x8a
	}

	header := bitmapInfoHeader{
		width:    iconSize,
		height:   -iconSize,
		planes:   1,
		bitCount: 32,
	}
	header.size = uint32(unsafe.Sizeof(header))

	var bits unsafe.Pointer
	hbmColor, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&header)),
		0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmColor == 0 || bits == nil {
		return 0
	}

	pixels := unsafe.Slice((*uint32)(bits), iconSize*iconSize)
	const center = (iconSize - 1) / 2.0
	const radius = iconSize/2 - 2

	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			dx := float64(x) - center
			dy := float64(y) - center
			d := dx*dx + dy*dy

			alpha := 0.0
			switch {
			case d <= float64((radius-1)*(radius-1)):
				alpha = 1
			case d <= float64(radius*radius):
				alpha = 0.5
			}

			a := byte(alpha * 255)
			pixels[y*iconSize+x] = uint32(a)<<24 |
				uint32(scale(r, alpha))<<16 |
				uint32(scale(g, alpha))<<8 |
				uint32(scale(b, alpha))
		}
	}

	hbmMask, _, _ := procCreateBitmap.Call(iconSize, iconSize, 1, 1, 0)
	info := iconInfo{fIcon: 1, hbmMask: hbmMask, hbmColor: hbmColor}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))

	procDeleteObject.Call(hbmColor)
	procDeleteObject.Call(hbmMask)
	return hicon
}

func scale(v byte, alpha float64) byte {
	return byte(float64(v) * alpha)
}
