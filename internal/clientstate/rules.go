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
	Title   string `json:"title,omitempty"`
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

func plainRole(role string) bool { return role == RoleDirect || role == RoleTunnel }

func (d *DB) RulesInForce() (string, []Rule, error) {
	rules, err := d.Rules()
	if err != nil {
		return "", nil, err
	}
	def, _ := d.DefaultRole()
	if sub, err := d.Subscription(); err == nil && sub.AllowExit {
		return def, rules, nil
	}
	if !plainRole(def) {
		def = RoleDirect
	}
	for i := range rules {
		if !plainRole(rules[i].Role) {
			rules[i].Role = def
		}
	}
	return def, rules, nil
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

func targetKey(process, path string) string {
	if path != "" {
		return "p:" + strings.ToLower(path)
	}
	return "n:" + strings.ToLower(process)
}

type DomainRule struct {
	ID      int    `json:"id"`
	Domain  string `json:"domain"`
	Role    string `json:"role"`
	Matched int    `json:"matched"`
}

func CleanDomain(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if at := strings.Index(name, "://"); at >= 0 {
		name = name[at+3:]
	}
	if at := strings.IndexAny(name, "/?#"); at >= 0 {
		name = name[:at]
	}
	name = strings.TrimPrefix(name, "*.")
	return strings.Trim(name, ". ")
}

func (d *DB) DomainRules() ([]DomainRule, error) {
	rows, err := d.sql.Query(`SELECT id, domain, role, matched FROM domain_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DomainRule{}
	for rows.Next() {
		var r DomainRule
		if err := rows.Scan(&r.ID, &r.Domain, &r.Role, &r.Matched); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) DomainRulesInForce() ([]DomainRule, error) {
	rules, err := d.DomainRules()
	if err != nil {
		return nil, err
	}
	if sub, err := d.Subscription(); err == nil && sub.AllowExit {
		return rules, nil
	}
	for i := range rules {
		if !plainRole(rules[i].Role) {
			rules[i].Role = RoleTunnel
		}
	}
	return rules, nil
}

func (d *DB) ReplaceDomainRules(rules []DomainRule) error {
	held, err := d.DomainRules()
	if err != nil {
		return err
	}
	matched := map[string]int{}
	for _, r := range held {
		matched[r.Domain] = r.Matched
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM domain_rules`); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range rules {
		name := CleanDomain(r.Domain)
		if !ValidRole(r.Role) {
			return fmt.Errorf("clientstate: unknown role %s for %s", r.Role, r.Domain)
		}
		if name == "" || !strings.Contains(name, ".") {
			return fmt.Errorf("clientstate: %q is not a domain", r.Domain)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, err := tx.Exec(`INSERT INTO domain_rules (domain, role, matched) VALUES (?, ?, ?)`,
			name, r.Role, matched[name]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) BundleRoles() (map[string]string, error) {
	rows, err := d.sql.Query(`SELECT bundle, role FROM bundle_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var bundle, role string
		if err := rows.Scan(&bundle, &role); err != nil {
			return nil, err
		}
		out[bundle] = role
	}
	return out, rows.Err()
}

func (d *DB) BundleRolesInForce() (map[string]string, error) {
	roles, err := d.BundleRoles()
	if err != nil {
		return nil, err
	}
	if sub, err := d.Subscription(); err == nil && sub.AllowExit {
		return roles, nil
	}
	for bundle, role := range roles {
		if !plainRole(role) {
			roles[bundle] = RoleTunnel
		}
	}
	return roles, nil
}

func (d *DB) ReplaceBundleRoles(roles map[string]string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM bundle_rules`); err != nil {
		return err
	}
	for bundle, role := range roles {
		if role == "" || role == RoleTunnel {
			continue
		}
		if !ValidRole(role) {
			return fmt.Errorf("clientstate: unknown role %s for %s", role, bundle)
		}
		if _, err := tx.Exec(`INSERT INTO bundle_rules (bundle, role) VALUES (?, ?)`, bundle, role); err != nil {
			return err
		}
	}
	return tx.Commit()
}
