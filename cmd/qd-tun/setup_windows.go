//go:build windows && !core

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/qcli/windivert"
	"github.com/jaywehosl/qd/internal/update"
)

const (
	arriveFlag   = "-after"
	leaveFlag    = "-uninstall"
	quitEvent    = `Global\qdClientQuit`
	uninstallKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\` + appName
	setupExe     = appName + ".exe"
	shortcutName = appName + ".lnk"
)

func init() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case leaveFlag:
		uninstall()
		os.Exit(0)
	case arriveFlag:
		if len(os.Args) > 2 {
			if pid, err := strconv.Atoi(os.Args[2]); err == nil {
				awaitExit(uint32(pid))
			}
			os.Args = append(os.Args[:1], os.Args[3:]...)
		}
		arrived = true
	}
}

func awaitExit(pid uint32) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if state, _ := windows.WaitForSingleObject(h, leaveWait); state != uint32(windows.WAIT_OBJECT_0) {
		windows.TerminateProcess(h, 0)
		windows.WaitForSingleObject(h, 5000)
	}
}

func setupDir() string {
	dir, err := windivert.DefaultDir()
	if err != nil {
		return ""
	}
	return filepath.Dir(dir)
}

func installed() bool {
	exe, err := os.Executable()
	dir := setupDir()
	return err == nil && dir != "" && strings.EqualFold(filepath.Dir(exe), dir)
}

type setup struct {
	mu  sync.Mutex
	db  *clientstate.DB
	tun *tunnel
}

func withSetup(routes http.Handler, db *clientstate.DB, tun *tunnel) http.Handler {
	if embedded {
		return routes
	}
	go mendAutostart()
	go heedQuit()
	if exe, err := os.Executable(); err == nil && installed() {
		enlist(exe)
	}

	s := &setup{db: db, tun: tun}
	mux := http.NewServeMux()
	mux.Handle("/", routes)
	mux.HandleFunc("/client/api/setup", s.state)
	mux.HandleFunc("/client/api/setup/install", s.install)
	return mux
}

func answer(w http.ResponseWriter, obj any, err error) {
	w.Header().Set("Content-Type", "application/json")
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	json.NewEncoder(w).Encode(map[string]any{"success": err == nil, "msg": msg, "obj": obj})
}

func (s *setup) view() map[string]any {
	in := installed()
	autostart := true
	if sub, err := s.db.Subscription(); err == nil && sub.Imported {
		settings, err := s.db.Settings()
		autostart = err == nil && settings.Autostart
	}
	return map[string]any{"installed": in, "offered": !in && paneDev == "", "path": setupDir(), "autostart": autostart}
}

func (s *setup) state(w http.ResponseWriter, r *http.Request) {
	answer(w, s.view(), nil)
}

func (s *setup) install(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Autostart bool `json:"autostart"`
		Desktop   bool `json:"desktop"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil {
		answer(w, nil, errors.New("post the two switches"))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.place(body.Autostart, body.Desktop); err != nil {
		fmt.Printf("setup    %v\n", err)
		answer(w, nil, err)
		return
	}
	answer(w, s.view(), nil)
	select {
	case updated <- struct{}{}:
	default:
	}
}

func (s *setup) place(autostart, desktop bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := setupDir()
	if dir == "" || installed() {
		return errors.New("this copy already runs from Program Files")
	}
	target := filepath.Join(dir, setupExe)

	body, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target+".new", body, 0o755); err != nil {
		return err
	}
	clearAside(target + ".old")
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, target+".old"); err != nil {
			os.Remove(target + ".new")
			return fmt.Errorf("the installed copy is in the way: %w", err)
		}
	}
	if err := os.Rename(target+".new", target); err != nil {
		return err
	}

	if err := enlist(target); err != nil {
		return err
	}
	menu, bench := shortcuts()
	if err := link(menu, target); err != nil {
		return err
	}
	if desktop {
		if err := link(bench, target); err != nil {
			return err
		}
	} else {
		os.Remove(bench)
	}

	if autostart {
		err = holdTask(autostartTask, target, "-autostart")
	} else {
		err = holdAutostart(false)
	}
	if err != nil {
		return err
	}
	if settings, err := s.db.Settings(); err == nil && settings.Autostart != autostart {
		settings.Autostart = autostart
		s.db.SaveSettings(settings)
	}
	if s.tun.Running() {
		s.db.SetValue(resumeKey, "1")
	}

	next := exec.Command(target, arriveFlag, strconv.Itoa(os.Getpid()))
	next.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	bequeath(next)
	if err := next.Start(); err != nil {
		return fmt.Errorf("the installed copy did not start: %w", err)
	}
	next.Process.Release()
	fmt.Printf("setup    installed into %s, the client restarts from there\n", dir)
	return nil
}

func enlist(exe string) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("the list of installed apps: %w", err)
	}
	defer key.Close()

	size := uint32(0)
	if info, err := os.Stat(exe); err == nil {
		size = uint32(info.Size() / 1024)
	}
	for name, value := range map[string]string{
		"DisplayName":     appName,
		"DisplayVersion":  strings.TrimPrefix(update.Version, "v"),
		"Publisher":       appName,
		"DisplayIcon":     exe,
		"InstallLocation": filepath.Dir(exe),
		"UninstallString": `"` + exe + `" ` + leaveFlag,
	} {
		if err := key.SetStringValue(name, value); err != nil {
			return fmt.Errorf("the list of installed apps: %w", err)
		}
	}
	key.SetDWordValue("NoModify", 1)
	key.SetDWordValue("NoRepair", 1)
	key.SetDWordValue("EstimatedSize", size)
	return nil
}

func shortcuts() (menu, bench string) {
	if dir, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0); err == nil {
		menu = filepath.Join(dir, shortcutName)
	}
	if dir, err := windows.KnownFolderPath(windows.FOLDERID_PublicDesktop, 0); err == nil {
		bench = filepath.Join(dir, shortcutName)
	}
	return menu, bench
}

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coCreateInstance = ole32.NewProc("CoCreateInstance")
	clsidShellLink   = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink     = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile   = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	comRelease        = 2
	comQuery          = 0
	shellLinkSetDir   = 9
	shellLinkSetPath  = 20
	persistFileSave   = 6
	clsctxInprocess   = 1
	comAlreadyStarted = syscall.Errno(1)
)

func comCall(object unsafe.Pointer, slot int, args ...uintptr) error {
	table := *(**[32]uintptr)(object)
	hr, _, _ := syscall.SyscallN(table[slot], append([]uintptr{uintptr(object)}, args...)...)
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

func link(path, target string) error {
	if path == "" {
		return errors.New("shortcut: windows names no folder for it")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil || err == comAlreadyStarted {
		defer windows.CoUninitialize()
	}

	var shell unsafe.Pointer
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, clsctxInprocess,
		uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&shell)))
	if int32(hr) < 0 || shell == nil {
		return fmt.Errorf("shortcut: %w", syscall.Errno(hr))
	}
	defer comCall(shell, comRelease)

	exe, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	dir, err := windows.UTF16PtrFromString(filepath.Dir(target))
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	if err := comCall(shell, shellLinkSetPath, uintptr(unsafe.Pointer(exe))); err != nil {
		return fmt.Errorf("shortcut: %w", err)
	}
	comCall(shell, shellLinkSetDir, uintptr(unsafe.Pointer(dir)))

	var disk unsafe.Pointer
	if err := comCall(shell, comQuery, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&disk))); err != nil || disk == nil {
		return fmt.Errorf("shortcut: %w", err)
	}
	defer comCall(disk, comRelease)
	if err := comCall(disk, persistFileSave, uintptr(unsafe.Pointer(file)), 1); err != nil {
		return fmt.Errorf("shortcut %s: %w", path, err)
	}
	return nil
}

func heedQuit() {
	handle := namedEvent(quitEvent, false)
	if handle == 0 {
		return
	}
	if state, err := windows.WaitForSingleObject(handle, windows.INFINITE); err != nil || state != uint32(windows.WAIT_OBJECT_0) {
		return
	}
	shutWindow()
	fmt.Printf("setup    windows is removing the client, leaving\n")
	select {
	case updated <- struct{}{}:
	default:
	}
}

func askQuit() {
	if name, err := windows.UTF16PtrFromString(quitEvent); err == nil {
		if handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name); err == nil {
			windows.SetEvent(handle)
			windows.CloseHandle(handle)
		}
	}
	mutex, err := windows.UTF16PtrFromString(instanceMutex)
	if err != nil {
		return
	}
	for i := 0; i < 48; i++ {
		held, err := windows.OpenMutex(windows.SYNCHRONIZE, false, mutex)
		if err != nil {
			return
		}
		windows.CloseHandle(held)
		time.Sleep(250 * time.Millisecond)
	}
}

func uninstall() {
	exe, err := os.Executable()
	dir := setupDir()
	if err != nil || dir == "" || !strings.EqualFold(filepath.Dir(exe), dir) {
		return
	}

	askQuit()
	shutWindow()
	endOthers()
	holdAutostart(false)
	menu, bench := shortcuts()
	os.Remove(menu)
	os.Remove(bench)
	registry.DeleteKey(registry.LOCAL_MACHINE, uninstallKey)
	releaseDriver()

	if root := os.Getenv("SystemRoot"); root != "" {
		aside := filepath.Join(root, "Temp")
		os.Chdir(aside)
		gone := filepath.Join(aside, fmt.Sprintf("%s-%d.gone", appName, os.Getpid()))
		if os.Rename(exe, gone) == nil {
			if name, err := windows.UTF16PtrFromString(gone); err == nil {
				windows.MoveFileEx(name, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
			}
		}
	}
	doomed := []string{dir}
	if profile, err := os.UserConfigDir(); err == nil {
		doomed = append(doomed, filepath.Join(profile, appName))
	}
	for i := 0; i < 20; i++ {
		left := 0
		for _, path := range doomed {
			if os.RemoveAll(path) != nil {
				left++
			}
		}
		if left == 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
}
