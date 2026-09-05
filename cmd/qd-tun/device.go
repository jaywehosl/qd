//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type device struct {
	ID       string `json:"device"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

func identify() device {
	vendor, product, serial := systemBIOS()

	parts := []string{}
	for _, p := range []string{machineGUID(), volumeSerial(), vendor, product, serial} {
		if p != "" {
			parts = append(parts, p)
		}
	}

	host, _ := os.Hostname()
	if len(parts) == 0 {
		parts = append(parts, host)
	}

	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))

	model := strings.TrimSpace(vendor + " " + product)
	if model == "" {
		model = "PC"
	}

	return device{
		ID:       hex.EncodeToString(sum[:])[:32],
		Platform: platformName(),
		Model:    model,
		Kind:     chassis(),
		Name:     host,
	}
}

type powerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func chassis() string {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")
	var status powerStatus
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return "desktop"
	}
	if status.BatteryFlag == 128 || status.BatteryFlag == 255 {
		return "desktop"
	}
	return "laptop"
}

func machineGUID() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer key.Close()

	guid, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return "guid:" + guid
}

func volumeSerial() string {
	root, err := windows.UTF16PtrFromString(os.Getenv("SystemDrive") + `\`)
	if err != nil {
		return ""
	}

	var serial uint32
	err = windows.GetVolumeInformation(root, nil, 0, &serial, nil, nil, nil, 0)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("vol:%08x", serial)
}

func systemBIOS() (vendor, product, serial string) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", ""
	}
	defer key.Close()

	vendor, _, _ = key.GetStringValue("SystemManufacturer")
	product, _, _ = key.GetStringValue("SystemProductName")
	serial, _, _ = key.GetStringValue("SystemSerialNumber")
	return strings.TrimSpace(vendor), strings.TrimSpace(product), strings.TrimSpace(serial)
}

func platformName() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer key.Close()

	name, _, _ := key.GetStringValue("ProductName")
	build, _, _ := key.GetStringValue("CurrentBuildNumber")

	if name == "" {
		return "Windows"
	}
	if n := parseBuild(build); n >= 22000 && strings.Contains(name, "Windows 10") {
		name = strings.Replace(name, "Windows 10", "Windows 11", 1)
	}
	if build != "" {
		return name + " (build " + build + ")"
	}
	return name
}

func parseBuild(text string) int {
	n := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
