package clientstate

import (
	"fmt"
	"strings"
)

const (
	RoleDirect   = "direct"
	RoleTunnel   = "tunnel"
	RoleEgress   = "egress"
	RoleNoEgress = "noEgress"
)

func ValidRole(role string) bool {
	switch role {
	case RoleDirect, RoleTunnel, RoleEgress, RoleNoEgress:
		return true
	}
	return false
}

type Rule struct {
	ID      int    `json:"id"`
	Process string `json:"process"`
	Path    string `json:"path,omitempty"`
	Role    string `json:"role"`
	Matched int    `json:"matched"`
	Running bool   `json:"running"`
	Icon    string `json:"icon,omitempty"`
}

func (d *DB) Rules() ([]Rule, error) {
	rows, err := d.sql.Query(`SELECT id, process, path, role, matched FROM rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Process, &r.Path, &r.Role, &r.Matched); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) DefaultRole() (string, error) {
	var v string
	err := d.sql.QueryRow(`SELECT value FROM settings WHERE key = 'defaultRole'`).Scan(&v)
	if err != nil || !ValidRole(v) {
		return RoleTunnel, nil
	}
	return v, nil
}

func (d *DB) ReplaceRules(defaultRole string, rules []Rule) error {
	if !ValidRole(defaultRole) {
		return fmt.Errorf("clientstate: %q is not a role", defaultRole)
	}

	existing, err := d.Rules()
	if err != nil {
		return err
	}
	byTarget := map[string]Rule{}
	for _, r := range existing {
		byTarget[targetKey(r.Process, r.Path)] = r
	}

	seen := map[string]bool{}
	kept := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if !ValidRole(r.Role) {
			return fmt.Errorf("clientstate: unknown role %s for %s", r.Role, r.Process)
		}
		if r.Process == "" {
			return fmt.Errorf("clientstate: a rule needs a process")
		}
		key := targetKey(r.Process, r.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if old, found := byTarget[key]; found {
			r.ID = old.ID
			r.Matched = old.Matched
		}
		kept = append(kept, r)
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM rules`); err != nil {
		return err
	}
	for _, r := range kept {
		if r.ID > 0 {
			if _, err := tx.Exec(
				`INSERT INTO rules (id, process, path, role, matched) VALUES (?, ?, ?, ?, ?)`,
				r.ID, r.Process, r.Path, r.Role, r.Matched); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO rules (process, path, role, matched) VALUES (?, ?, ?, 0)`,
			r.Process, r.Path, r.Role); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO settings (key, value) VALUES ('defaultRole', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, defaultRole); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) BumpRule(id int) error {
	_, err := d.sql.Exec(`UPDATE rules SET matched = matched + 1 WHERE id = ?`, id)
	return err
}

func targetKey(process, path string) string {
	if path != "" {
		return "p:" + strings.ToLower(path)
	}
	return "n:" + strings.ToLower(process)
}
