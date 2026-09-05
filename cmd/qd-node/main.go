//go:build linux

package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/qsrv"
	"github.com/jaywehosl/quic-diver/internal/store"
)

var version = "dev"

func main() {
	iface := flag.String("iface", "", "interface, empty = autodetect by default route")
	dbPath := flag.String("db", "node.db", "where this node keeps the network database")

	confFlag := flag.String("config", configPath, "where this node keeps its identity")

	showVersion := flag.Bool("version", false, "print the version and exit")
	doInit := flag.Bool("init", false, "write a first-node database and exit")

	wantHelp := flag.Bool("help", false, "what this node can be asked to do")
	helpInit := flag.Bool("help-init", false, "arguments of -init")
	wantStatus := flag.Bool("status", false, "what this node is and what it sees")
	wantRestart := flag.Bool("restart", false, "restart the service")
	listAdmins := flag.Bool("admins", false, "list the administrators of this network")
	addAdmin := flag.String("admin-add", "", "add an administrator and print its link")
	addAdminGroup := flag.String("admin-group", "", "-admin-add: group for the new administrator, empty = the first one")
	certHook := flag.Bool("cert-hook", false, "rewrite the certbot renewal hook")
	var opts initOptions
	flag.StringVar(&opts.networkKey, "key", "", "-init: network key to join, empty = mint one and start a network")
	flag.IntVar(&opts.port, "port", store.DefaultPort, "-init: udp port this node answers on")
	flag.StringVar(&opts.address, "address", "", "-init: address clients dial, empty = autodetect")
	flag.StringVar(&opts.role, "role", string(netstate.RoleIngress), "-init: ingress or egress")
	flag.IntVar(&opts.nodeID, "node-id", 0, "-init: number the panel gave this node, 0 = let the database choose")
	flag.StringVar(&opts.nodeTag, "tag", "", "-init: name of this node, empty = pick from the pool")
	flag.StringVar(&opts.nodeUUID, "node-uuid", "", "-init: uuid of this node, empty = mint one")
	flag.StringVar(&opts.adminTag, "admin", "", "-init: tag of the administrator")
	flag.StringVar(&opts.adminUUID, "admin-uuid", "", "-init: uuid of the administrator, empty = mint one")
	flag.StringVar(&opts.groupTag, "group", "", "-init: tag of the first group, empty = create none")
	flag.StringVar(&opts.authority, "authority", "", "-init: host:port this node answers as")
	flag.StringVar(&opts.certPath, "cert", "", "-init: certificate for this node")
	flag.StringVar(&opts.keyPath, "key-file", "", "-init: private key for this node")
	flag.StringVar(&opts.dnsPrimary, "dns1", "1.1.1.1", "-init: first upstream resolver")
	flag.StringVar(&opts.dnsSecondary, "dns2", "8.8.8.8", "-init: second upstream resolver")
	flag.IntVar(&opts.dnsCache, "dns-cache", 4096, "-init: names held in the resolver cache")
	flag.IntVar(&opts.dnsMinTTL, "dns-min-ttl", 60, "-init: lower ttl bound, seconds")
	flag.IntVar(&opts.dnsMaxTTL, "dns-max-ttl", 3600, "-init: upper ttl bound, seconds")
	flag.IntVar(&opts.dnsStale, "dns-stale", 60, "-init: serve-stale window, seconds")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *doInit {
		opts.configAt = *confFlag
		if err := runInit(*dbPath, *iface, opts); err != nil {
			fatal("init: %v", err)
		}
		return
	}
	if *wantHelp {
		showHelp()
		return
	}
	if *helpInit {
		showInitHelp()
		return
	}
	if *wantRestart {
		if err := runRestart(); err != nil {
			fatal("restart: %v", err)
		}
		return
	}

	conf, err := readConfig(*confFlag)
	if err != nil {
		fatal("config %s: %v", *confFlag, err)
	}

	if *wantStatus {
		if err := runStatus(conf, *dbPath); err != nil {
			fatal("status: %v", err)
		}
		return
	}
	if *listAdmins {
		if err := runAdmins(conf, *dbPath); err != nil {
			fatal("admins: %v", err)
		}
		return
	}
	if *addAdmin != "" {
		if err := runAdminAdd(conf, *dbPath, *addAdmin, *addAdminGroup); err != nil {
			fatal("admin-add: %v", err)
		}
		return
	}
	if *certHook {
		if err := runCertHook(conf); err != nil {
			fatal("cert-hook: %v", err)
		}
		return
	}
	if conf.DB != "" && *dbPath == "node.db" {
		*dbPath = conf.DB
	}
	want := conf.identity()
	if want.UUID == "" {
		want.UUID = readSelfUUID(*dbPath)
	}

	logs := captureOutput(4000)

	dev, err := defaultDevice(*iface)
	if err != nil {
		fatal("%v", err)
	}

	link, err := net.InterfaceByName(dev)
	if err != nil {
		fatal("interface %s: %v", dev, err)
	}

	serverIP, err := primaryIPv4(link)
	if err != nil {
		fatal("%v", err)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		fatal("database: %v", err)
	}
	defer db.Close()

	now := time.Now().UnixMilli()

	key, err := db.NetworkKey(now)
	if err != nil {
		fatal("network key: %v", err)
	}

	self, err := db.SelfNode(want, serverIP.String(), now)
	if err != nil {
		fatal("database: %v", err)
	}

	if err := db.SelfEntrypoint(self, now); err != nil {
		fatal("database: %v", err)
	}

	settings, err := db.NetworkSettings()
	if err != nil {
		fatal("settings: %v", err)
	}

	var netKey qdcrypt.Key
	raw, err := hex.DecodeString(key)
	if err != nil || len(raw) != qdcrypt.KeySize {
		log.Fatalf("network key must be %d hex chars", qdcrypt.KeySize*2)
	}
	copy(netKey[:], raw)

	host := self.Authority
	if host == "" {
		host = self.Address
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	authority := net.JoinHostPort(host, strconv.Itoa(self.Port))

	certPath, keyPath := self.CertPath, self.KeyPath
	if missing := missingFile(certPath, keyPath); missing != "" {
		fmt.Printf("tls        %s is not here, standing up on a self-signed certificate\n", missing)
		certPath, keyPath = "", ""
	}

	tlsConf, err := loadTLS(certPath, keyPath, authority)
	if err != nil {
		fatal("tls: %v", err)
	}

	fmt.Printf("version    %s\n", version)
	fmt.Printf("interface  %s\n", dev)
	fmt.Printf("server ip  %s\n", serverIP)
	fmt.Printf("database   %s, this node is #%d (%s, %s)\n", *dbPath, self.ID, self.Tag, self.Role)
	if !self.Enable {
		fmt.Printf("carrying   off, it answers the panel and takes no client traffic\n")
	}
	if certPath == "" {
		fmt.Printf("tls        self-signed, clients pin the certificate\n")
	} else {
		fmt.Printf("tls        %s\n", certPath)
	}

	admission := newGate()
	admission.setNetwork(key)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var state *controlState

	node := qsrv.New(qsrv.Config{
		Listen:    fmt.Sprintf(":%d", self.Port),
		Authority: authority,
		SelfID:    self.UUID,
		SelfTag:   self.Tag,
		Pool:      poolOf(settings.Pool),
		TLS:       tlsConf,
		Token:     key,
		Verify:    admission.verify,
		Peers:     peersFrom(db, self.ID),
		Tune:      func() qsrv.Tunables { return tunablesFrom(mustSettings(db)) },
		Ask: func(op string, body []byte, auth string) (any, error) {
			return askNode(state, op, body, auth)
		},
		Log: func(format string, args ...any) { log.Printf(format, args...) },
	})

	watch := watchPresence(func() (map[uint32]seen, error) { return sampleSessions(node) })

	state = &controlState{
		tag: self.Tag, role: string(self.Role), address: self.Address, uuid: self.UUID,
		enable: self.Enable, key: key, id: self.ID, port: self.Port,
		db: db, dbPath: *dbPath, started: time.Now(),
		netKey:  &netKey,
		metrics: startMetrics(dev, watch.Live),
		logs:    logs,
		watch:   watch,
		epoch:   time.Now().Unix(),
		node:    node,
		sessions: &sessionMap{
			add:   func(id uint32) error { admission.add(id); return nil },
			del:   func(id uint32) error { admission.del(id); node.Forget(id); return nil },
			exit:  func(id uint32, allow bool) error { admission.exit(id, allow); return nil },
			reset: func(id uint32) error { node.Reset(id); return nil },
			list:  func() (map[uint32]struct{}, error) { return admission.list(), nil },
			stat: func() ([]sessionStat, error) {
				live, err := sampleSessions(node)
				if err != nil {
					return nil, err
				}

				out := make([]sessionStat, 0, len(live))
				for id, s := range live {
					since, lastSeen, checked, fingerprint, addresses := watch.of(id)
					out = append(out, sessionStat{
						Session: id, Client: s.Client, Transit: s.Transit, LastSeen: lastSeen,
						Since: since, Checked: checked, Device: fingerprint, Seen: addresses,
						Up: s.Up, Down: s.Down, PktUp: s.PktUp, PktDown: s.PktDown,
					})
				}
				return out, nil
			},
		},
		restart: func() {
			db.Close()
			binary, err := os.Executable()
			if err != nil {
				binary = os.Args[0]
			}
			syscall.Exec(binary, os.Args, os.Environ())
		},
	}

	if mine := state.mySession(); mine != 0 {
		fmt.Printf("identity   session %d, the number peer nodes know this one by\n", mine)
	} else {
		fmt.Printf("identity   none yet, the panel has not given this node a secret\n")
	}
	state.applySelf()
	state.syncSessions()
	state.startResolver()
	go state.recordTelemetry()

	fmt.Printf("quic       udp/%d, authority %s, pool %s\n", self.Port, authority, settings.Pool)
	if settings.BrutalMbit > 0 {
		os.Setenv("QD_BRUTAL_MBPS", fmt.Sprint(settings.BrutalMbit))
		fmt.Printf("congestion brutal, %d Mbit/s regardless of loss\n", settings.BrutalMbit)
	} else {
		fmt.Printf("congestion cubic\n")
	}
	fmt.Println()

	if settings.StatsSeconds > 0 {
		go func() {
			t := time.NewTicker(time.Duration(settings.StatsSeconds) * time.Second)
			defer t.Stop()
			for range t.C {
				printStats(node)
			}
		}()
	}

	go node.Remember(ctx, *dbPath+".nat46")
	go reportSelf(ctx, db, self.ID)
	go runNode(ctx, node)

	<-ctx.Done()
	fmt.Println("\nstopping")
	printStats(node)
}

func mustSettings(db *store.DB) store.NetworkSettings {
	s, err := db.NetworkSettings()
	if err != nil {
		return store.NetworkSettings{}
	}
	return s
}

func printStats(node *qsrv.Node) {
	sessions, transits, refused := node.Live()
	fmt.Printf("sessions=%d transits=%d refused=%d\n", sessions, transits, refused)

	for _, s := range node.Sessions() {
		if s.PktUp == 0 && s.PktDown == 0 {
			continue
		}
		fmt.Printf("  session %d  %s  out %d pkt / %s  in %d pkt / %s\n",
			s.Session, s.Address, s.PktUp, human(s.Up), s.PktDown, human(s.Down))
	}
}

func human(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func defaultDevice(want string) (string, error) {
	if want != "" {
		return want, nil
	}
	out, err := exec.Command("ip", "route", "get", "8.8.8.8").Output()
	if err != nil {
		return "", fmt.Errorf("ip route get: %w", err)
	}
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("cannot determine outbound interface")
}

func primaryIPv4(link *net.Interface) (netip.Addr, error) {
	addrs, err := link.Addrs()
	if err != nil {
		return netip.Addr{}, err
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ipnet.IP.To4()
		if v4 == nil {
			continue
		}
		addr, ok := netip.AddrFromSlice(v4)
		if ok && !addr.IsLoopback() {
			return addr, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("no ipv4 address on %s", link.Name)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func sampleSessions(node *qsrv.Node) (map[uint32]seen, error) {
	out := map[uint32]seen{}
	for _, s := range node.Sessions() {
		where := s.Peer
		if where == "" {
			where = s.Address.Addr().String()
		}

		out[s.Session] = seen{
			Client:   where,
			Transit:  s.Transit,
			LastSeen: s.LastSeen * 1000,
			Up:       s.Up,
			Down:     s.Down,
			PktUp:    s.PktUp,
			PktDown:  s.PktDown,
		}
	}
	return out, nil
}

func missingFile(paths ...string) string {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			return path
		}
	}
	return ""
}
