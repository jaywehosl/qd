//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const (
	autostartTask = appName
	taskNoLimit   = "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>"
)

func taskOf(name string) (string, bool) {
	out, err := run("schtasks", "/query", "/tn", name, "/xml")
	return out, err == nil && strings.Contains(out, "<Task")
}

func autostartHeld() bool {
	_, held := taskOf(autostartTask)
	return held
}

func holdAutostart(on bool) error {
	if !on {
		out, err := run("schtasks", "/delete", "/tn", autostartTask, "/f")
		if err != nil && autostartHeld() {
			return fmt.Errorf("autostart: %s", strings.TrimSpace(out))
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return holdTask(autostartTask, exe, "-autostart")
}

func mendAutostart() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	held, ok := taskOf(autostartTask)
	if !ok || strings.Contains(held, taskNoLimit) || !strings.Contains(strings.ToLower(held), strings.ToLower(html.EscapeString(exe))) {
		return
	}
	if err := holdTask(autostartTask, exe, "-autostart"); err != nil {
		fmt.Printf("autostart %v\n", err)
		return
	}
	fmt.Printf("autostart the logon task no longer stops on battery or after three days\n")
}

func holdTask(name, exe, args string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	sid := user.User.Sid.String()

	text := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + sid + `</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>` + sid + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    ` + taskNoLimit + `
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + html.EscapeString(exe) + `</Command><Arguments>` + html.EscapeString(args) + `</Arguments></Exec></Actions>
</Task>`

	raw := []byte{0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune(text)) {
		raw = binary.LittleEndian.AppendUint16(raw, unit)
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("qd-task-%d.xml", os.Getpid()))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	defer os.Remove(path)

	out, err := run("schtasks", "/create", "/tn", name, "/xml", path, "/f")
	if err != nil {
		return fmt.Errorf("autostart: %s", strings.TrimSpace(out))
	}
	return nil
}
