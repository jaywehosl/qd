//go:build linux

package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/store"
)

const (
	renewHook   = "/etc/letsencrypt/renewal-hooks/deploy/qd-node.sh"
	serviceName = "qd-node"
)

func showHelp() {
	fmt.Print(`qd node.

Running it with no arguments starts the node: it reads its identity from the
config file and the rest of what it needs from the network database.

  -config PATH        where this node keeps its identity, default /etc/qd/node.conf
  -db PATH            network database, default: the one named in the config
  -version            print the version and exit
  -help               this text

Bringing a node up:
  -init               write a first-node database and exit; see -help-init

Running it afterwards:
  -status             what this node is, whether it serves, and what it sees
  -restart            restart the service without touching the machine
  -admins             list the administrators this network has
  -admin-add TAG      add an administrator and print the link that reaches it
  -cert-hook          rewrite the certbot hook that restarts the node on renewal

Every command that changes something says what it changed.
`)
}

func showInitHelp() {
	fmt.Print(`qd node -init writes a database and a config, then exits.

The installer calls it; you rarely need it by hand.

  -key HEX            network key to join; empty mints one and starts a network
  -address HOST       domain clients dial, also the name on the certificate
  -port N             udp port
  -role ingress|egress
  -tag NAME           name of this node, empty picks one from the pool
  -node-uuid UUID     identity of this node, empty mints one
  -node-id N          number the panel gave it, 0 lets the database choose
  -admin TAG          tag of the administrator
  -admin-uuid UUID    uuid of the administrator, empty mints one
  -group TAG          first group, only when starting a network
  -authority HOST:PORT
  -cert PATH -key-file PATH
  -dns1 A -dns2 B -dns-cache N -dns-min-ttl N -dns-max-ttl N -dns-stale N
`)
}

func cliDatabase(cfg nodeConfig, dbFlag string) (*store.DB, string, error) {
	path := dbFlag
	if cfg.DB != "" && (path == "" || path == "node.db") {
		path = cfg.DB
	}
	if path == "" {
		return nil, "", fmt.Errorf("no database path: pass -db or fix %s", configPath)
	}
	db, err := store.Open(path)
	if err != nil {
		return nil, path, err
	}
	return db, path, nil
}

func runAdmins(cfg nodeConfig, dbFlag string) error {
	db, path, err := cliDatabase(cfg, dbFlag)
	if err != nil {
		return err
	}
	defer db.Close()

	clients, err := db.Clients()
	if err != nil {
		return err
	}
	groups, err := db.Groups()
	if err != nil {
		return err
	}
	named := map[int]string{}
	for _, g := range groups {
		named[g.ID] = g.Tag
	}

	fmt.Printf("database   %s\n\n", path)
	found := 0
	for _, c := range clients {
		if !c.Admin {
			continue
		}
		found++
		state := "enabled"
		if !c.Enable {
			state = "disabled"
		}
		group := named[c.GroupID]
		if group == "" {
			group = "no group"
		}
		fmt.Printf("  %-20s %s  %s, %s\n", c.Tag, c.UUID, state, group)
	}
	if found == 0 {
		fmt.Printf("  no administrator in this database; -admin-add makes one\n")
	}
	return nil
}

func runAdminAdd(cfg nodeConfig, dbFlag, tag, groupTag string) error {
	if err := checkTag("admin tag", tag, true); err != nil {
		return err
	}

	db, path, err := cliDatabase(cfg, dbFlag)
	if err != nil {
		return err
	}
	defer db.Close()

	clients, err := db.Clients()
	if err != nil {
		return err
	}
	for _, c := range clients {
		if strings.EqualFold(c.Tag, tag) {
			return fmt.Errorf("%q is already here; pick another tag", tag)
		}
	}

	groups, err := db.Groups()
	if err != nil {
		return err
	}
	groupID := 0
	switch {
	case groupTag != "":
		for _, g := range groups {
			if strings.EqualFold(g.Tag, groupTag) {
				groupID = g.ID
			}
		}
		if groupID == 0 {
			return fmt.Errorf("no group called %q", groupTag)
		}
	case len(groups) > 0:
		sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
		groupID = groups[0].ID
	}

	uuid, err := newUUID()
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	if _, err := db.SaveClient(netstate.Client{
		Tag:     tag,
		UUID:    uuid,
		GroupID: groupID,
		Enable:  true,
		Admin:   true,
	}, now); err != nil {
		return err
	}
	revision, err := db.Touch(now)
	if err != nil {
		return err
	}

	key, err := db.NetworkKey(now)
	if err != nil {
		return err
	}

	host, port := cfg.Address, cfg.Port
	if host == "" || port == 0 {
		nodes, err := db.Nodes()
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if n.ID == cfg.ID || host == "" {
				host, port = n.Address, n.Port
				break
			}
		}
	}
	if host == "" || port == 0 {
		return fmt.Errorf("this database knows no address to dial; run this on a node")
	}

	link := clientstate.Link{
		Key:        uuid,
		Label:      tag,
		NetworkKey: key,
		Endpoints:  []clientstate.Endpoint{{Address: host, Port: port}},
	}.String()

	group := "no group"
	for _, g := range groups {
		if g.ID == groupID {
			group = g.Tag
		}
	}

	fmt.Printf("database   %s\n", path)
	fmt.Printf("admin      %s, %s\n", tag, group)
	fmt.Printf("uuid       %s\n", uuid)
	fmt.Printf("revision   %d, the panel hands it to the other nodes\n\n", revision)
	fmt.Printf("%s\n\n", link)
	fmt.Printf("Import that into the client. Restart this node so it admits the new admin:\n")
	fmt.Printf("  qd-node -restart\n")
	return nil
}

func runRestart() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("no systemctl here, restart the process by hand")
	}
	fmt.Printf("restarting %s\n", serviceName)
	out, err := exec.Command("systemctl", "restart", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl: %s", strings.TrimSpace(string(out)))
	}
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		if serviceActive() {
			fmt.Printf("ok         %s is running again\n", serviceName)
			return nil
		}
	}
	return fmt.Errorf("%s did not come back; journalctl -u %s -n 30", serviceName, serviceName)
}

func serviceActive() bool {
	out, _ := exec.Command("systemctl", "is-active", serviceName).Output()
	return strings.TrimSpace(string(out)) == "active"
}

func certificateName(cfg nodeConfig) string {
	if cfg.Cert != "" {
		dir := filepath.Base(filepath.Dir(cfg.Cert))
		if dir != "" && dir != "." && dir != "/" {
			return dir
		}
	}
	host := cfg.Authority
	if host == "" {
		host = cfg.Address
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host
}

func runCertHook(cfg nodeConfig) error {
	name := certificateName(cfg)
	if name == "" {
		return fmt.Errorf("this node has no domain in %s, nothing to hook", configPath)
	}

	live := filepath.Join("/etc/letsencrypt/live", name)
	if _, err := os.Stat(filepath.Join(live, "fullchain.pem")); err != nil {
		return fmt.Errorf("no certificate in %s; issue one with certbot first", live)
	}

	if err := os.MkdirAll(filepath.Dir(renewHook), 0o755); err != nil {
		return err
	}
	body := "#!/bin/sh\nsystemctl restart " + serviceName + " 2>/dev/null || true\n"
	if err := os.WriteFile(renewHook, []byte(body), 0o755); err != nil {
		return err
	}

	fmt.Printf("domain     %s\n", name)
	fmt.Printf("certificate %s\n", live)
	fmt.Printf("hook       %s\n", renewHook)
	fmt.Printf("ok         renewal restarts %s from now on\n", serviceName)
	return nil
}

func certificateEnds(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	left := time.Until(crt.NotAfter)
	return fmt.Sprintf("%s, %d days left", crt.NotAfter.UTC().Format("2006-01-02"), int(left.Hours()/24))
}

type ears struct {
	udp []int
	tcp []int
}

func (e ears) quiet() bool { return len(e.udp) == 0 && len(e.tcp) == 0 }

func (e ears) holds(port int) bool {
	for _, p := range append(append([]int{}, e.udp...), e.tcp...) {
		if p == port {
			return true
		}
	}
	return false
}

func listeningNow() ears {
	held := socketsOfService()
	if len(held) == 0 {
		return ears{}
	}
	return ears{
		udp: portsHeld(held, []string{"/proc/net/udp", "/proc/net/udp6"}, ""),
		tcp: portsHeld(held, []string{"/proc/net/tcp", "/proc/net/tcp6"}, "0A"),
	}
}

func socketsOfService() map[string]bool {
	out := map[string]bool{}
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, dir := range dirs {
		comm, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err != nil || strings.TrimSpace(string(comm)) != serviceName {
			continue
		}
		fds, _ := filepath.Glob(filepath.Join(dir, "fd", "*"))
		for _, fd := range fds {
			link, err := os.Readlink(fd)
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			out[strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")] = true
		}
	}
	return out
}

func portsHeld(held map[string]bool, files []string, state string) []int {
	seen := map[int]bool{}
	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 10 || !held[fields[9]] {
				continue
			}
			if state != "" && fields[3] != state {
				continue
			}
			at := strings.LastIndex(fields[1], ":")
			if at < 0 {
				continue
			}
			port, err := strconv.ParseUint(fields[1][at+1:], 16, 32)
			if err != nil {
				continue
			}
			seen[int(port)] = true
		}
	}
	out := make([]int, 0, len(seen))
	for port := range seen {
		out = append(out, port)
	}
	sort.Ints(out)
	return out
}

func spellPorts(ports []int) string {
	if len(ports) == 0 {
		return "nothing"
	}
	words := make([]string, len(ports))
	for i, p := range ports {
		words[i] = strconv.Itoa(p)
	}
	return strings.Join(words, ", ")
}

func mark(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func selfFromDatabase(cfg nodeConfig, db *store.DB) netstate.Node {
	nodes, err := db.Nodes()
	if err != nil {
		return netstate.Node{}
	}
	for _, n := range nodes {
		if (cfg.UUID != "" && n.UUID == cfg.UUID) || (cfg.ID != 0 && n.ID == cfg.ID) {
			return n
		}
	}
	return netstate.Node{}
}

func runStatus(cfg nodeConfig, dbFlag string) error {
	self := netstate.Node{}
	db, path, err := cliDatabase(cfg, dbFlag)
	if err == nil {
		self = selfFromDatabase(cfg, db)
		defer db.Close()
	}

	id := self.ID
	if id == 0 {
		id = cfg.ID
	}
	uuid := self.UUID
	if uuid == "" {
		uuid = cfg.UUID
	}
	certPath := self.CertPath
	if certPath == "" {
		certPath = cfg.Cert
	}

	heard := listeningNow()
	answers := self.Authority
	if answers == "" {
		answers = self.Address
	}
	if host, _, cut := net.SplitHostPort(answers); cut == nil {
		answers = host
	}
	if answers != "" && self.Port > 0 {
		answers = net.JoinHostPort(answers, strconv.Itoa(self.Port))
	}

	fmt.Printf("\nidentity\n")
	fmt.Printf("  node       %s, %s, id %d\n", orNone(self.Tag), orNone(string(self.Role)), id)
	fmt.Printf("  uuid       %s\n", orNone(uuid))
	fmt.Printf("  answers as %s\n", orNone(answers))
	fmt.Printf("  address    %s\n", orNone(self.Address))
	fmt.Printf("  database   %s\n", orNone(path))

	fmt.Printf("\nservice\n")
	active := serviceActive()
	fmt.Printf("  running    %s\n", mark(active))
	fmt.Printf("  udp        %s\n", spellPorts(heard.udp))
	fmt.Printf("  tcp        %s\n", spellPorts(heard.tcp))
	if self.Port > 0 && !heard.quiet() && !heard.holds(self.Port) {
		fmt.Printf("  mismatch   the network says this node is on %d, it listens elsewhere\n", self.Port)
	}

	if certPath == "" {
		fmt.Printf("  certificate self-signed\n")
	} else if ends := certificateEnds(certPath); ends != "" {
		fmt.Printf("  certificate %s\n", ends)
		hook := "missing"
		if _, err := os.Stat(renewHook); err == nil {
			hook = "in place"
		}
		fmt.Printf("  renew hook %s\n", hook)
	} else {
		fmt.Printf("  certificate %s is unreadable\n", certPath)
	}

	if err != nil {
		fmt.Printf("\ndatabase\n  %v\n\n", err)
		return nil
	}

	fmt.Printf("\nnetwork\n")

	version, err := db.Version()
	if err == nil {
		fmt.Printf("  revision   %d\n", version)
	}

	nodes, _ := db.Nodes()
	clients, _ := db.Clients()
	groups, _ := db.Groups()
	admins := 0
	for _, c := range clients {
		if c.Admin && c.Enable {
			admins++
		}
	}
	fmt.Printf("  knows      %d nodes, %d groups, %d clients, %d admins\n",
		len(nodes), len(groups), len(clients), admins)

	seen := ""
	if progress, err := db.NodeProgress(); err == nil {
		if p, ok := progress[id]; ok {
			if p.LastSeen > 0 {
				ago := time.Since(time.UnixMilli(p.LastSeen)).Round(time.Second)
				seen = fmt.Sprintf("%s ago, applied revision %d, %s", ago, p.Applied, p.Status)
			} else {
				seen = fmt.Sprintf("applied revision %d, %s", p.Applied, p.Status)
			}
		}
	}
	if seen == "" {
		seen = "the panel has not written anything about this node yet"
	}
	fmt.Printf("  panel says %s\n", seen)

	if admins == 0 {
		fmt.Printf("\n  no enabled administrator: nobody can open the panel.\n")
		fmt.Printf("  qd-node -admin-add TAG makes one and prints its link.\n")
	}
	if !active {
		fmt.Printf("\n  the service is not running: systemctl start %s\n", serviceName)
	}
	fmt.Printf("\n")
	return nil
}

func orNone(text string) string {
	if text == "" {
		return "(none)"
	}
	return text
}

func reportSelf(ctx context.Context, db *store.DB, nodeID int) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()

	for {
		version, err := db.Version()
		if err == nil {
			db.RecordNodeProgress(nodeID, version, version, "online", time.Now().UnixMilli())
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
