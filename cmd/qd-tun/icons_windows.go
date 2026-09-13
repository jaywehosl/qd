//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32x = windows.NewLazySystemDLL("user32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")

	shGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	getIconInfo    = user32x.NewProc("GetIconInfo")
	destroyIcon    = user32x.NewProc("DestroyIcon")

	getDCx             = user32x.NewProc("GetDC")
	releaseDC          = user32x.NewProc("ReleaseDC")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	deleteDC           = gdi32.NewProc("DeleteDC")
	deleteObject       = gdi32.NewProc("DeleteObject")
	getObjectW         = gdi32.NewProc("GetObjectW")
	getDIBits          = gdi32.NewProc("GetDIBits")
)

const (
	shgfiIcon      = 0x000000100
	shgfiLargeIcon = 0x000000000
	biRGB          = 0
	dibRGBColors   = 0
)

type shFileInfoW struct {
	Icon        windows.Handle
	Index       int32
	Attributes  uint32
	DisplayName [windows.MAX_PATH]uint16
	TypeName    [80]uint16
}

type iconInfoW struct {
	IsIcon   int32
	XHotspot uint32
	YHotspot uint32
	MaskBmp  windows.Handle
	ColorBmp windows.Handle
}

type bitmapW struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

var iconCache struct {
	mu   sync.Mutex
	seen map[string]string
}

func processIcon(path string) string {
	if path == "" {
		return ""
	}

	iconCache.mu.Lock()
	if iconCache.seen == nil {
		iconCache.seen = map[string]string{}
	}
	if held, ok := iconCache.seen[path]; ok {
		iconCache.mu.Unlock()
		return held
	}
	iconCache.mu.Unlock()

	uri := readIcon(path)

	iconCache.mu.Lock()
	iconCache.seen[path] = uri
	iconCache.mu.Unlock()
	return uri
}

func readIcon(path string) string {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}

	var info shFileInfoW
	ret, _, _ := shGetFileInfoW.Call(
		uintptr(unsafe.Pointer(wide)), 0,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		shgfiIcon|shgfiLargeIcon)
	if ret == 0 || info.Icon == 0 {
		return ""
	}
	defer destroyIcon.Call(uintptr(info.Icon))

	img := iconToImage(info.Icon)
	if img == nil {
		return ""
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func iconToImage(icon windows.Handle) image.Image {
	var ii iconInfoW
	if ret, _, _ := getIconInfo.Call(uintptr(icon), uintptr(unsafe.Pointer(&ii))); ret == 0 {
		return nil
	}
	if ii.ColorBmp != 0 {
		defer deleteObject.Call(uintptr(ii.ColorBmp))
	}
	if ii.MaskBmp != 0 {
		defer deleteObject.Call(uintptr(ii.MaskBmp))
	}
	if ii.ColorBmp == 0 {
		return nil
	}

	var bmp bitmapW
	if ret, _, _ := getObjectW.Call(uintptr(ii.ColorBmp), unsafe.Sizeof(bmp),
		uintptr(unsafe.Pointer(&bmp))); ret == 0 {
		return nil
	}
	if bmp.Width <= 0 || bmp.Height <= 0 || bmp.Width > 512 || bmp.Height > 512 {
		return nil
	}

	colour := readBits(ii.ColorBmp, bmp.Width, bmp.Height)
	if colour == nil {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, int(bmp.Width), int(bmp.Height)))
	opaque := true
	for _, px := range colour {
		if px>>24 != 0 {
			opaque = false
			break
		}
	}

	var mask []uint32
	if opaque && ii.MaskBmp != 0 {
		mask = readBits(ii.MaskBmp, bmp.Width, bmp.Height)
	}

	for y := 0; y < int(bmp.Height); y++ {
		for x := 0; x < int(bmp.Width); x++ {
			px := colour[y*int(bmp.Width)+x]
			a := uint8(px >> 24)
			if opaque {
				a = 0xff
				if mask != nil && mask[y*int(bmp.Width)+x]&0x00ffffff != 0 {
					a = 0
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(px >> 16),
				G: uint8(px >> 8),
				B: uint8(px),
				A: a,
			})
		}
	}
	return img
}

func readBits(bmp windows.Handle, width, height int32) []uint32 {
	screen, _, _ := getDCx.Call(0)
	if screen == 0 {
		return nil
	}
	defer releaseDC.Call(0, screen)

	dc, _, _ := createCompatibleDC.Call(screen)
	if dc == 0 {
		return nil
	}
	defer deleteDC.Call(dc)

	head := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       width,
		Height:      -height,
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}

	out := make([]uint32, width*height)
	ret, _, _ := getDIBits.Call(dc, uintptr(bmp), 0, uintptr(height),
		uintptr(unsafe.Pointer(&out[0])), uintptr(unsafe.Pointer(&head)), dibRGBColors)
	if ret == 0 {
		return nil
	}
	return out
}
