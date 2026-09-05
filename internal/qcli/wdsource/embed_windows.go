//go:build windows

package windivert

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/WinDivert.dll assets/WinDivert64.sys
var assets embed.FS

func DefaultDir() (string, error) {
	appData, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appData, "QUICDiver"), nil
}

func Extract(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("создать %s: %w", dir, err)
	}
	var dllPath string
	for _, name := range []string{"WinDivert.dll", "WinDivert64.sys"} {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return "", fmt.Errorf("вшитый %s: %w", name, err)
		}
		path := filepath.Join(dir, name)
		if err := writeIfDiffers(path, data); err != nil {
			return "", err
		}
		if name == "WinDivert.dll" {
			dllPath = path
		}
	}
	return dllPath, nil
}

func writeIfDiffers(path string, want []byte) error {
	if have, err := os.ReadFile(path); err == nil &&
		sha256.Sum256(have) == sha256.Sum256(want) {
		return nil
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		return fmt.Errorf("записать %s: %w", path, err)
	}
	return nil
}
