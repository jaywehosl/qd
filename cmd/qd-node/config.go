//go:build linux

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jaywehosl/quic-diver/internal/netstate"
)

const configPath = "/etc/qd/node.conf"

type nodeConfig struct {
	DB        string
	Authority string
	Cert      string
	Key       string

	ID      int
	UUID    string
	Tag     string
	Role    string
	Address string
	Port    int
}

func readConfig(path string) (nodeConfig, error) {
	var cfg nodeConfig

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.Trim(strings.TrimSpace(value), `"`)

		switch name {
		case "db":
			cfg.DB = value
		case "authority":
			cfg.Authority = value
		case "cert":
			cfg.Cert = value
		case "key":
			cfg.Key = value
		case "id":
			cfg.ID, _ = strconv.Atoi(value)
		case "uuid":
			cfg.UUID = value
		case "tag":
			cfg.Tag = value
		case "role":
			cfg.Role = value
		case "address":
			cfg.Address = value
		case "port":
			cfg.Port, _ = strconv.Atoi(value)
		}
	}
	return cfg, lines.Err()
}

func (c nodeConfig) identity() netstate.Node {
	role := netstate.Role(c.Role)
	if role != netstate.RoleEgress {
		role = netstate.RoleIngress
	}
	return netstate.Node{
		ID:        c.ID,
		Tag:       c.Tag,
		Address:   c.Address,
		Port:      c.Port,
		Role:      role,
		Enable:    true,
		UUID:      c.UUID,
		Authority: c.Authority,
		CertPath:  c.Cert,
		KeyPath:   c.Key,
	}
}

func writeConfig(path string, cfg nodeConfig) error {
	if path == "" {
		path = configPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	body := strings.Builder{}
	for _, line := range [][2]string{
		{"db", cfg.DB},
		{"id", strconv.Itoa(cfg.ID)},
		{"uuid", cfg.UUID},
		{"tag", cfg.Tag},
		{"role", cfg.Role},
		{"address", cfg.Address},
		{"port", strconv.Itoa(cfg.Port)},
		{"authority", cfg.Authority},
		{"cert", cfg.Cert},
		{"key", cfg.Key},
	} {
		if line[1] == "" || line[1] == "0" {
			continue
		}
		body.WriteString(line[0] + " = " + line[1] + "\n")
	}
	return os.WriteFile(path, []byte(body.String()), 0o600)
}
