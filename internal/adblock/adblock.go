package adblock

import (
	_ "embed"
	"strings"
)

//go:embed adblocklist.txt
var defaultList string

type List struct {
	blocked map[string]struct{}
}

func Default() *List {
	l := &List{blocked: map[string]struct{}{}}
	for _, field := range strings.FieldsFunc(defaultList, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		if name := normalize(field); name != "" {
			l.blocked[name] = struct{}{}
		}
	}
	return l
}

func (l *List) Blocked(name string) bool {
	if l == nil || len(l.blocked) == 0 {
		return false
	}
	name = normalize(name)
	for name != "" {
		if _, hit := l.blocked[name]; hit {
			return true
		}
		dot := strings.IndexByte(name, '.')
		if dot < 0 {
			return false
		}
		name = name[dot+1:]
	}
	return false
}

func normalize(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
}
