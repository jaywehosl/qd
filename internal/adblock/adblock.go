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

func New() *List {
	return &List{blocked: make(map[string]struct{})}
}

func Default() *List {
	l := New()
	l.Add(defaultList)
	return l
}

func (l *List) Add(text string) int {
	added := 0
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		name := normalize(field)
		if name == "" {
			continue
		}
		if _, seen := l.blocked[name]; seen {
			continue
		}
		l.blocked[name] = struct{}{}
		added++
	}
	return added
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

func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.blocked)
}

func normalize(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
}
