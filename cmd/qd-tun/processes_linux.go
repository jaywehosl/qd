//go:build linux

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func snapshotProcesses() []processInfo {
	out := []processInfo{}
	for _, name := range dirNames("/proc") {
		pid, err := strconv.ParseUint(name, 10, 32)
		if err != nil {
			continue
		}
		ident := lookupProcess(uint32(pid))
		if ident.name == "" {
			continue
		}
		out = append(out, processInfo{
			Name: ident.name, Path: ident.path, Icon: appIcon(uint32(pid), ident), PID: int(pid),
		})
	}
	return out
}

type appIndex struct {
	byID    map[string]string
	byExec  map[string]string
	byClass map[string]string
}

var icons struct {
	mu    sync.Mutex
	at    time.Time
	index appIndex
	data  map[string]string
}

func appIcon(pid uint32, ident procIdent) string {
	icons.mu.Lock()
	defer icons.mu.Unlock()

	if time.Since(icons.at) > time.Minute {
		icons.index = indexApps()
		icons.data = map[string]string{}
		icons.at = time.Now()
	}

	idx := icons.index
	base := strings.ToLower(ident.name)
	name := first(idx.byExec[base], idx.byID[base], idx.byClass[base])
	if name == "" {
		if id := scopeApp(pid); id != "" {
			name = first(idx.byID[strings.ToLower(id)], idx.byClass[strings.ToLower(id)])
		}
	}
	if name == "" {
		return ""
	}
	if url, ok := icons.data[name]; ok {
		return url
	}
	url := iconData(name)
	icons.data[name] = url
	return url
}

func first(options ...string) string {
	for _, o := range options {
		if o != "" {
			return o
		}
	}
	return ""
}

func indexApps() appIndex {
	idx := appIndex{byID: map[string]string{}, byExec: map[string]string{}, byClass: map[string]string{}}
	dirs := []string{
		"/usr/share/applications", "/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications", "/var/lib/snapd/desktop/applications",
	}
	for _, pattern := range []string{
		"/home/*/.local/share/applications", "/home/*/.local/share/flatpak/exports/share/applications",
	} {
		found, _ := filepath.Glob(pattern)
		dirs = append(dirs, found...)
	}

	for _, dir := range dirs {
		for _, file := range dirNames(dir) {
			id, ok := strings.CutSuffix(file, ".desktop")
			if !ok {
				continue
			}
			exec, icon, class := readDesktop(filepath.Join(dir, file))
			if icon == "" {
				continue
			}
			keep(idx.byID, strings.ToLower(id), icon)
			keep(idx.byExec, execName(exec), icon)
			keep(idx.byClass, strings.ToLower(class), icon)
		}
	}
	return idx
}

func keep(m map[string]string, key, icon string) {
	if key == "" {
		return
	}
	if _, held := m[key]; !held {
		m[key] = icon
	}
}

func readDesktop(path string) (exec, icon, class string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	inEntry := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Exec":
			exec = strings.TrimSpace(value)
		case "Icon":
			icon = strings.TrimSpace(value)
		case "StartupWMClass":
			class = strings.TrimSpace(value)
		}
	}
	return
}

var launchers = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "env": true,
	"python": true, "python3": true, "perl": true, "java": true, "gjs": true,
}

func execName(exec string) string {
	fields := strings.Fields(exec)
	for i, f := range fields {
		f = strings.Trim(f, `"'`)
		if f == "env" || (strings.Contains(f, "=") && !strings.HasPrefix(f, "/")) {
			continue
		}
		base := strings.ToLower(filepath.Base(f))
		if base == "flatpak" {
			for _, g := range fields[i+1:] {
				if cmd, ok := strings.CutPrefix(g, "--command="); ok {
					return strings.ToLower(filepath.Base(cmd))
				}
			}
			return ""
		}
		if launchers[base] {
			return ""
		}
		return base
	}
	return ""
}

func scopeApp(pid uint32) string {
	raw, err := os.ReadFile("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/cgroup")
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(raw)), "\n")
	unit := filepath.Base(line)

	if rest, ok := strings.CutPrefix(unit, "snap."); ok {
		snap, app, ok := strings.Cut(rest, ".")
		if !ok {
			return ""
		}
		app, _, _ = strings.Cut(app, "-")
		return snap + "_" + app
	}

	rest, ok := strings.CutPrefix(unit, "app-")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, ".scope"), ".service")
	if at := strings.IndexByte(rest, '@'); at >= 0 {
		rest = rest[:at]
	} else if dash := strings.LastIndexByte(rest, '-'); dash >= 0 {
		rest = rest[:dash]
	}
	if _, id, ok := strings.Cut(rest, "-"); ok {
		rest = id
	}
	return strings.ReplaceAll(rest, `\x2d`, "-")
}

func iconData(name string) string {
	path := name
	if !filepath.IsAbs(name) {
		path = findIcon(name)
	}
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 256<<10 {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	case ".svg":
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(raw)
	}
	return ""
}

func findIcon(name string) string {
	roots := []string{
		"/usr/share/icons/hicolor", "/usr/local/share/icons/hicolor",
		"/var/lib/flatpak/exports/share/icons/hicolor",
	}
	for _, pattern := range []string{
		"/home/*/.local/share/icons/hicolor", "/home/*/.local/share/flatpak/exports/share/icons/hicolor",
	} {
		found, _ := filepath.Glob(pattern)
		roots = append(roots, found...)
	}

	for _, root := range roots {
		for _, size := range []string{"48x48", "64x64", "96x96", "128x128", "scalable", "256x256", "32x32", "512x512"} {
			for _, ext := range []string{".png", ".svg"} {
				if p := filepath.Join(root, size, "apps", name+ext); exists(p) {
					return p
				}
			}
		}
	}
	for _, p := range []string{name, name + ".png", name + ".svg"} {
		if p = filepath.Join("/usr/share/pixmaps", p); exists(p) {
			return p
		}
	}
	return ""
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
