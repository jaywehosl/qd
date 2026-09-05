//go:build windows

package main

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmNCCalcSize = 0x0083
	wmNCHitTest  = 0x0084

	htClient      = 1
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	gwlpWndProc = ^uintptr(3)

	smCXFrame        = 32
	smCYFrame        = 33
	smCXPaddedBorder = 92

	swpFrameChanged = 0x0020
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
)

var (
	setWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	callWindowProc   = user32.NewProc("CallWindowProcW")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	getWindowRect    = user32.NewProc("GetWindowRect")
	isZoomedCall     = user32.NewProc("IsZoomed")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	setWindowPos     = user32.NewProc("SetWindowPos")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type frame struct {
	mu     sync.Mutex
	handle uintptr
	prev   uintptr
	bar    int32
	holes  []rect
}

var chrome frame

func (f *frame) barHeight() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bar
}

func (f *frame) setBar(height int32) {
	f.mu.Lock()
	f.bar = height
	f.mu.Unlock()
}

func (f *frame) setHoles(list []rect) {
	f.mu.Lock()
	f.holes = list
	f.mu.Unlock()
}

func (f *frame) inHole(x, y int32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.holes {
		if x >= h.Left && x < h.Right && y >= h.Top && y < h.Bottom {
			return true
		}
	}
	return false
}

func metric(index int32) int32 {
	value, _, _ := getSystemMetrics.Call(uintptr(index))
	return int32(value)
}

func (f *frame) take(handle uintptr) {
	if handle == 0 {
		return
	}

	f.mu.Lock()
	f.handle = handle
	if f.bar == 0 {
		f.bar = 64
	}
	f.mu.Unlock()

	prev, _, _ := setWindowLongPtr.Call(handle, gwlpWndProc,
		windows.NewCallback(chromeProc))

	f.mu.Lock()
	f.prev = prev
	f.mu.Unlock()

	shadow(handle)
	setWindowPos.Call(handle, 0, 0, 0, 0, 0,
		uintptr(swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder))
}

func chromeProc(handle uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmNCCalcSize:
		if wParam == 0 {
			break
		}
		// Client area takes the whole window, so the system caption is gone —
		// but a maximised window still has to keep the border out, or it spills
		// past the screen edges by the frame thickness.
		if zoomed, _, _ := isZoomedCall.Call(handle); zoomed != 0 {
			edge := metric(smCXFrame) + metric(smCXPaddedBorder)
			top := metric(smCYFrame) + metric(smCXPaddedBorder)
			area := (*rect)(unsafe.Pointer(lParam))
			area.Left += edge
			area.Right -= edge
			area.Top += top
			area.Bottom -= edge
		}
		return 0

	case wmNCHitTest:
		return chrome.hit(handle, lParam)
	}

	chrome.mu.Lock()
	prev := chrome.prev
	chrome.mu.Unlock()

	if prev != 0 {
		result, _, _ := callWindowProc.Call(prev, handle, uintptr(message), wParam, lParam)
		return result
	}
	result, _, _ := defWindowProc.Call(handle, uintptr(message), wParam, lParam)
	return result
}

func (f *frame) hit(handle uintptr, lParam uintptr) uintptr {
	x := int32(int16(lParam & 0xFFFF))
	y := int32(int16((lParam >> 16) & 0xFFFF))

	var box rect
	getWindowRect.Call(handle, uintptr(unsafe.Pointer(&box)))

	grip := metric(smCXFrame) + metric(smCXPaddedBorder)
	if grip < 6 {
		grip = 6
	}

	zoomed, _, _ := isZoomedCall.Call(handle)
	if zoomed == 0 {
		left := x < box.Left+grip
		right := x >= box.Right-grip
		top := y < box.Top+grip
		bottom := y >= box.Bottom-grip

		switch {
		case top && left:
			return htTopLeft
		case top && right:
			return htTopRight
		case bottom && left:
			return htBottomLeft
		case bottom && right:
			return htBottomRight
		case left:
			return htLeft
		case right:
			return htRight
		case top:
			return htTop
		case bottom:
			return htBottom
		}
	}

	if y < box.Top+f.barHeight() && !f.inHole(x-box.Left, y-box.Top) {
		return htCaption
	}
	return htClient
}

const (
	scMinimise = 0xF020
	scMaximise = 0xF030
	scRestore  = 0xF120
	scClose    = 0xF060
	wmSysCmd   = 0x0112
)

func command(handle uintptr, what string) {
	if handle == 0 {
		return
	}
	send := user32.NewProc("SendMessageW")

	switch what {
	case "minimise":
		send.Call(handle, uintptr(wmSysCmd), uintptr(scMinimise), 0)
	case "maximise":
		if zoomed, _, _ := isZoomedCall.Call(handle); zoomed != 0 {
			send.Call(handle, uintptr(wmSysCmd), uintptr(scRestore), 0)
			return
		}
		send.Call(handle, uintptr(wmSysCmd), uintptr(scMaximise), 0)
	case "close":
		send.Call(handle, uintptr(wmSysCmd), uintptr(scClose), 0)
	}
}

const wmNCLButtonDown = 0x00A1

var (
	releaseCapture = user32.NewProc("ReleaseCapture")
	sendMessage    = user32.NewProc("SendMessageW")
)

func grab(handle uintptr, edge string) {
	if handle == 0 {
		return
	}

	where := map[string]uintptr{
		"caption":     htCaption,
		"left":        htLeft,
		"right":       htRight,
		"top":         htTop,
		"bottom":      htBottom,
		"topleft":     htTopLeft,
		"topright":    htTopRight,
		"bottomleft":  htBottomLeft,
		"bottomright": htBottomRight,
	}[edge]
	if where == 0 {
		return
	}

	releaseCapture.Call()
	sendMessage.Call(handle, uintptr(wmNCLButtonDown), where, 0)
}

type margins struct{ Left, Right, Top, Bottom int32 }

var extendFrame = dwmapi.NewProc("DwmExtendFrameIntoClientArea")

func shadow(handle uintptr) {
	if handle == 0 {
		return
	}
	edge := margins{Left: 0, Right: 0, Top: 1, Bottom: 0}
	extendFrame.Call(handle, uintptr(unsafe.Pointer(&edge)))
}
