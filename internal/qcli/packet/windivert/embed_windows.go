//go:build windows

package windivert

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// WinDivert вшит в бинарь: релизный клиент — один .exe без внешних зависимостей.
// При запуске файлы распаковываются в рабочую папку (%APPDATA%\QUICDiver), откуда
// и грузятся: WinDivert.dll сама поднимает драйвер из лежащего рядом .sys,
// поэтому оба файла обязаны быть в одном каталоге.
//
//go:embed assets/WinDivert.dll assets/WinDivert64.sys
var assets embed.FS

// DefaultDir — рабочая папка клиента (%APPDATA%\QUICDiver).
func DefaultDir() (string, error) {
	appData, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appData, "QUICDiver"), nil
}

// Extract раскладывает вшитый WinDivert в dir и возвращает путь к DLL.
// Существующие файлы перезаписываются только при несовпадении содержимого —
// иначе загруженный драйвер держал бы файл и запись падала бы.
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
		return nil // уже актуален
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		return fmt.Errorf("записать %s: %w", path, err)
	}
	return nil
}
