//go:build windows

package windivert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "WinDivert"

func driverBeside(dllPath string) string {
	return filepath.Join(filepath.Dir(dllPath), "WinDivert64.sys")
}

func held(s *mgr.Service) string {
	cfg, err := s.Config()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(cfg.BinaryPathName, `\??\`)
}

func Mend(dllPath string) string {
	m, err := mgr.Connect()
	if err != nil {
		return ""
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return ""
	}
	defer s.Close()

	if state, err := s.Query(); err != nil || state.State != svc.Stopped {
		return ""
	}
	path := held(s)
	if _, err := os.Stat(path); err == nil && strings.EqualFold(path, driverBeside(dllPath)) {
		return ""
	}
	if err := s.Delete(); err != nil {
		return ""
	}
	return path
}

func Unload(dllPaths ...string) bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	defer s.Close()

	ours := false
	for _, dll := range dllPaths {
		ours = ours || strings.EqualFold(held(s), driverBeside(dll))
	}
	if !ours {
		return false
	}
	_, err = s.Control(svc.Stop)
	return err == nil
}

func Describe(dllPath string) string {
	sys := driverBeside(dllPath)
	file := "present"
	if _, err := os.Stat(sys); err != nil {
		file = "missing"
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Sprintf("driver file %s, service manager refused: %v", file, err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Sprintf("driver file %s, no driver service registered", file)
	}
	defer s.Close()

	state := "unknown"
	if got, err := s.Query(); err == nil {
		state = map[svc.State]string{
			svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping",
			svc.Running: "running",
		}[got.State]
	}
	return fmt.Sprintf("driver file %s, service %s from %s", file, state, held(s))
}
