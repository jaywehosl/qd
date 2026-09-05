//go:build linux

package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaywehosl/quic-diver/internal/clientstate"
	"github.com/jaywehosl/quic-diver/internal/netstate"
	"github.com/jaywehosl/quic-diver/internal/qdcrypt"
	"github.com/jaywehosl/quic-diver/internal/store"
)

type initOptions struct {
	networkKey string
	port       int
	address    string
	role       string
	nodeID     int
	nodeTag    string
	nodeUUID   string
	adminTag   string
	adminUUID  string
	groupTag   string
	authority  string
	certPath   string
	keyPath    string
	configAt   string

	dnsPrimary   string
	dnsSecondary string
	dnsCache     int
	dnsMinTTL    int
	dnsMaxTTL    int
	dnsStale     int
}

var typedTag = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func runInit(dbPath, iface string, opts initOptions) error {
	first := opts.networkKey == ""

	if opts.port < 1 || opts.port > 65535 {
		return fmt.Errorf("port %d is outside 1..65535", opts.port)
	}

	role := netstate.Role(opts.role)
	if role != netstate.RoleIngress && role != netstate.RoleEgress {
		return fmt.Errorf("role %q is neither ingress nor egress", opts.role)
	}
	if first && role != netstate.RoleIngress {
		return errors.New("the first node of a network is always an ingress")
	}

	if err := checkTag("admin tag", opts.adminTag, first); err != nil {
		return err
	}
	if first {
		if err := checkTag("group tag", opts.groupTag, true); err != nil {
			return err
		}
	} else if opts.groupTag != "" {
		if err := checkTag("group tag", opts.groupTag, false); err != nil {
			return err
		}
	}
	if opts.nodeTag != "" {
		if err := checkTag("node tag", opts.nodeTag, false); err != nil {
			return err
		}
	}

	if !first {
		if _, err := hex.DecodeString(opts.networkKey); err != nil || len(opts.networkKey) != qdcrypt.KeySize*2 {
			return fmt.Errorf("the network key must be %d hex characters", qdcrypt.KeySize*2)
		}
		if opts.adminUUID == "" {
			return errors.New("a node joining a network needs the uuid of an administrator")
		}
	}

	address := opts.address
	if address == "" {
		found, err := detectAddress(iface)
		if err != nil {
			return err
		}
		address = found
	}
	if parsed, err := netip.ParseAddr(address); err == nil && !parsed.IsGlobalUnicast() {
		return fmt.Errorf("%s is not an address clients can dial, pass -address", address)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := freshDatabase(db); err != nil {
		return err
	}

	now := time.Now().UnixMilli()

	key := opts.networkKey
	if first {
		key, err = db.NetworkKey(now)
		if err != nil {
			return err
		}
	} else if err := db.SetNetworkKey(key, now); err != nil {
		return err
	}

	settings, err := db.NetworkSettings()
	if err != nil {
		return err
	}
	settings.DNSPrimary = opts.dnsPrimary
	settings.DNSSecondary = opts.dnsSecondary
	settings.DNSCache = opts.dnsCache
	settings.DNSMinTTL = opts.dnsMinTTL
	settings.DNSMaxTTL = opts.dnsMaxTTL
	settings.DNSStale = opts.dnsStale
	if err := db.SaveNetworkSettings(settings); err != nil {
		return err
	}

	nodeUUID := opts.nodeUUID
	if nodeUUID == "" {
		if nodeUUID, err = newUUID(); err != nil {
			return err
		}
	}
	adminUUID := opts.adminUUID
	if adminUUID == "" {
		if adminUUID, err = newUUID(); err != nil {
			return err
		}
	}

	tag := opts.nodeTag
	if tag == "" {
		tag = netstate.PickName(role, map[string]bool{})
	}

	node := netstate.Node{
		Tag:     tag,
		Address: address,
		Port:    opts.port,
		Role:    role,
		Enable:  true,
		UUID:    nodeUUID,
	}
	nodeID := opts.nodeID
	if nodeID > 0 {
		node.ID = nodeID
		if err := db.InsertNodeAt(node, now); err != nil {
			return err
		}
	} else {
		nodeID, err = db.SaveNode(node, now)
		if err != nil {
			return err
		}
		node.ID = nodeID
	}

	entryID, err := db.SaveEntrypoint(netstate.Entrypoint{
		NodeID: nodeID,
		Port:   opts.port,
		Remark: node.Tag,
		Enable: true,
	}, now)
	if err != nil {
		return err
	}

	groupID := 0
	if opts.groupTag != "" {
		members := []int{}
		if role == netstate.RoleIngress {
			members = append(members, entryID)
		}
		groupID, err = db.SaveGroup(netstate.Group{
			Tag:           opts.groupTag,
			EntrypointIDs: members,
		}, now)
		if err != nil {
			return err
		}
	}

	if _, err := db.SaveClient(netstate.Client{
		Tag:     opts.adminTag,
		UUID:    adminUUID,
		GroupID: groupID,
		Enable:  true,
		Admin:   true,
	}, now); err != nil {
		return err
	}

	if err := writeSelfUUID(dbPath, nodeUUID); err != nil {
		return err
	}

	if err := writeConfig(opts.configAt, nodeConfig{
		DB:        dbPath,
		Authority: opts.authority,
		Cert:      opts.certPath,
		Key:       opts.keyPath,
		ID:        node.ID,
		UUID:      nodeUUID,
		Tag:       node.Tag,
		Role:      string(role),
		Address:   address,
		Port:      opts.port,
	}); err != nil {
		return err
	}

	fmt.Printf("node-id=%d\n", node.ID)
	fmt.Printf("node-tag=%s\n", node.Tag)
	fmt.Printf("node-uuid=%s\n", nodeUUID)
	fmt.Printf("node-address=%s\n", address)
	fmt.Printf("node-port=%d\n", opts.port)
	fmt.Printf("node-role=%s\n", role)
	fmt.Printf("admin-tag=%s\n", opts.adminTag)
	fmt.Printf("admin-uuid=%s\n", adminUUID)

	if first {
		fmt.Printf("network-key=%s\n", key)
		fmt.Printf("link=%s\n", clientstate.Link{
			Key:        adminUUID,
			Label:      opts.adminTag,
			NetworkKey: key,
			Endpoints:  []clientstate.Endpoint{{Address: address, Port: opts.port}},
		}.String())
	}
	return nil
}

func selfUUIDPath(dbPath string) string { return dbPath + ".self" }

func writeSelfUUID(dbPath, uuid string) error {
	return os.WriteFile(selfUUIDPath(dbPath), []byte(uuid+"\n"), 0o600)
}

func readSelfUUID(dbPath string) string {
	raw, err := os.ReadFile(selfUUIDPath(dbPath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func checkTag(what, value string, typed bool) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", what)
	}
	if typed {
		if !typedTag.MatchString(value) {
			return fmt.Errorf("%s %q: letters, digits, dash and underscore only, up to 32", what, value)
		}
		return nil
	}
	if utf8.RuneCountInString(value) > 64 {
		return fmt.Errorf("%s %q is longer than 64 characters", what, value)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s %q carries control characters", what, value)
	}
	return nil
}

func freshDatabase(db *store.DB) error {
	nodes, err := db.Nodes()
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		return fmt.Errorf("this database already describes %d node(s)", len(nodes))
	}

	var key string
	err = db.SQL().QueryRow(`SELECT key FROM network WHERE id = 1`).Scan(&key)
	if err == nil {
		return errors.New("this database already carries a network key")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func detectAddress(iface string) (string, error) {
	dev, err := defaultDevice(iface)
	if err != nil {
		return "", err
	}
	link, err := net.InterfaceByName(dev)
	if err != nil {
		return "", err
	}
	ip, err := primaryIPv4(link)
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
