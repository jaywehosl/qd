package steerlist

import (
	_ "embed"
	"net/netip"
	"strings"

	"github.com/jaywehosl/qd/internal/clientstate"
	"github.com/jaywehosl/qd/internal/domainroute"
)

type Service struct {
	ID      string
	Name    string
	Entries []string
	Nets    []netip.Prefix
}

//go:embed names.txt
var names string

//go:embed nets.txt
var nets string

func listed(text string) map[string][]string {
	held := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		if id, rest, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			held[id] = strings.Fields(rest)
		}
	}
	return held
}

func init() {
	byName, byNet := listed(names), listed(nets)
	for i := range Services {
		Services[i].Entries = byName[Services[i].ID]
		for _, text := range byNet[Services[i].ID] {
			if p, err := netip.ParsePrefix(text); err == nil {
				Services[i].Nets = append(Services[i].Nets, p.Masked())
			}
		}
	}
}

type Bundle struct {
	ID      string
	Name    string
	Members []string
}

const BundleMark = "#"

func (b Bundle) Services() []Service {
	out := make([]Service, 0, len(b.Members))
	for _, id := range b.Members {
		for _, s := range Services {
			if s.ID == id {
				out = append(out, s)
			}
		}
	}
	return out
}

func Lower(roles map[string]string) map[string]domainroute.Entry {
	out := map[string]domainroute.Entry{}
	for _, b := range Bundles {
		role := roles[b.ID]
		if role == "" {
			role = clientstate.RoleTunnel
		}
		for _, s := range b.Services() {
			for _, name := range s.Entries {
				out[name] = domainroute.Entry{Rule: BundleMark + b.ID, Role: role}
			}
		}
	}
	return out
}

func Nets(roles map[string]string) []domainroute.Net {
	var out []domainroute.Net
	for _, b := range Bundles {
		role := roles[b.ID]
		if role == "" {
			role = clientstate.RoleTunnel
		}
		for _, s := range b.Services() {
			for _, p := range s.Nets {
				out = append(out, domainroute.Net{Prefix: p, Entry: domainroute.Entry{Rule: BundleMark + b.ID, Role: role}})
			}
		}
	}
	return out
}
