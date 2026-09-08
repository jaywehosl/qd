// Package clientdns — резолвер клиента: слушает свой UDP-порт и отвечает,
// спрашивая узел по управляющему каналу.
//
// Один на все клиенты намеренно. Раньше их было два, под Windows и под Android,
// с одинаковыми Serve, handle и keepWarm и разошедшимися мелочами — и правка в
// одном до другого не доезжала.
package clientdns

import (
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaywehosl/quic-diver/internal/dnsproxy"
)

// Ask — как спросить узел. Транспорт у клиентов свой, вопрос один.
type Ask func(endpoint, op, auth string, body, out any) error

type Config struct {
	Node  string
	Token string
	Ask   Ask
	// Blocked — какие имена отклонять; ставится до первого запроса, иначе
	// первые имена проскочат мимо списка.
	Blocked func(name string) bool
	// Say — куда писать про каждый запрос; nil означает молчать.
	Say func(format string, args ...any)
	// Keep — сколько последних запросов помнить для интерфейса; 0 — не помнить.
	Keep int
}

type Stats struct {
	Queries, Hits, Upstream, Failed, Blocked, NoV6 uint64
}

// Query — след одного запроса, каким его показывает интерфейс клиента.
type Query struct {
	Name  string `json:"name"`
	Ms    int64  `json:"nodeMs"`
	Whole int64  `json:"wholeMs"`
	Kind  string `json:"kind,omitempty"`
	Hit   bool   `json:"hit"`
	Err   string `json:"err,omitempty"`
	At    int64  `json:"sinceUpMs"`
}

type Resolver struct {
	conn  *net.UDPConn
	node  atomic.Pointer[string]
	token string
	ask   Ask
	say   func(string, ...any)
	keep  int
	born  time.Time

	mu      sync.Mutex
	blocked func(name string) bool
	recent  []Query

	queries, hits, upstream, failed, refused, noV6 atomic.Uint64
}

func New(cfg Config) (*Resolver, error) {
	if cfg.Ask == nil {
		return nil, errors.New("clientdns: no way to ask the node")
	}
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	r := &Resolver{
		conn: conn, token: cfg.Token, ask: cfg.Ask,
		say: cfg.Say, keep: cfg.Keep, born: time.Now(),
	}
	r.blocked = cfg.Blocked
	r.node.Store(&cfg.Node)
	return r, nil
}

func (r *Resolver) Addr() string { return r.conn.LocalAddr().String() }

// SetNode переводит резолвер на узел, выигравший гонку: спрашивать первый вход
// подписки незачем, туннель мог подняться совсем через другой.
func (r *Resolver) SetNode(node string) {
	if node == "" {
		return
	}
	r.node.Store(&node)
}

// SetBlocked задаёт, какие имена отклонять. Ставится после создания: список
// блокировок клиент собирает позже резолвера.
func (r *Resolver) SetBlocked(fn func(name string) bool) {
	r.mu.Lock()
	r.blocked = fn
	r.mu.Unlock()
}

func (r *Resolver) Serve(stop <-chan struct{}) {
	buf := make([]byte, 4096)

	for {
		select {
		case <-stop:
			return
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(readStep))
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil || n < 12 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])
		go r.handle(query, from)
	}
}

const readStep = 500 * time.Millisecond

func (r *Resolver) Close() {
	if r != nil && r.conn != nil {
		r.conn.Close()
	}
}

// Interrupt снимает ожидание в Serve немедленно, не дожидаясь шага чтения.
func (r *Resolver) Interrupt() {
	if r != nil && r.conn != nil {
		r.conn.SetReadDeadline(time.Now())
	}
}

func (r *Resolver) handle(query []byte, from *net.UDPAddr) {
	entered := time.Now()
	r.queries.Add(1)

	name, qtype, ok := dnsproxy.Question(query)
	if !ok {
		r.failed.Add(1)
		return
	}

	if r.blocks(name) {
		r.refused.Add(1)
		r.conn.WriteToUDP(dnsproxy.Refused(query), from)
		r.note(Query{Name: name, Kind: "blocked", Whole: since(entered), At: since(r.born)})
		return
	}

	// Клиент живёт по IPv4 внутри туннеля: спрашивать AAAA незачем, а пустой
	// ответ отправляет систему к записи A немедленно, без ожидания таймаута.
	if qtype == 28 {
		r.noV6.Add(1)
		r.conn.WriteToUDP(dnsproxy.NoData(query), from)
		return
	}

	began := time.Now()
	answer, hit, err := r.fetch(query)
	spent := since(began)
	r.tell("dns: %s %dms hit=%v err=%v", name, spent, hit, err)

	if err != nil {
		r.failed.Add(1)
		// Молчание стоило бы приложению целого таймаута резолвера. Отказ доходит
		// сразу, и система идёт дальше.
		r.conn.WriteToUDP(dnsproxy.ServFail(query), from)
		r.note(Query{Name: name, Ms: spent, Whole: since(entered), Err: err.Error(), At: since(r.born)})
		return
	}

	if hit {
		r.hits.Add(1)
	} else {
		r.upstream.Add(1)
	}
	r.conn.WriteToUDP(answer, from)
	r.note(Query{Name: name, Ms: spent, Whole: since(entered), Hit: hit, At: since(r.born)})
}

func (r *Resolver) fetch(query []byte) ([]byte, bool, error) {
	var answer struct {
		Answer []byte `json:"answer"`
		Hit    bool   `json:"hit"`
	}
	if err := r.ask(r.asking(), "dns", r.token, map[string]any{"query": query}, &answer); err != nil {
		return nil, false, err
	}
	if len(answer.Answer) < 12 {
		return nil, false, errors.New("the node returned nothing")
	}

	copy(answer.Answer[0:2], query[0:2])
	return answer.Answer, answer.Hit, nil
}

// KeepWarm держит управляющий канал живым между запросами: узел иначе закрывает
// его по молчанию, и первое же имя после паузы ждало бы нового рукопожатия.
func (r *Resolver) KeepWarm(stop <-chan struct{}) {
	tick := time.NewTicker(warmStep)
	defer tick.Stop()

	for {
		if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
			r.tell("dns: the node went quiet between queries: %v", err)
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

const warmStep = 20 * time.Second

// RTT меряет задержку до узла тем же вопросом, которым проверяется живость:
// одно обращение отвечает сразу на оба.
func (r *Resolver) RTT() int {
	began := time.Now()
	if err := r.ask(r.asking(), "whoami", r.token, nil, nil); err != nil {
		return -1
	}
	return int(since(began))
}

func (r *Resolver) Stats() Stats {
	return Stats{
		Queries:  r.queries.Load(),
		Hits:     r.hits.Load(),
		Upstream: r.upstream.Load(),
		Failed:   r.failed.Load(),
		Blocked:  r.refused.Load(),
		NoV6:     r.noV6.Load(),
	}
}

func (r *Resolver) RecentJSON() string {
	r.mu.Lock()
	out := append([]Query{}, r.recent...)
	r.mu.Unlock()

	blob, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(blob)
}

func (r *Resolver) asking() string {
	if held := r.node.Load(); held != nil {
		return *held
	}
	return ""
}

func (r *Resolver) blocks(name string) bool {
	r.mu.Lock()
	fn := r.blocked
	r.mu.Unlock()
	return fn != nil && fn(name)
}

func (r *Resolver) note(q Query) {
	if r.keep <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recent = append(r.recent, q)
	if len(r.recent) > r.keep {
		r.recent = append([]Query{}, r.recent[len(r.recent)-r.keep:]...)
	}
}

func (r *Resolver) tell(format string, args ...any) {
	if r.say != nil {
		r.say(format, args...)
	}
}

func since(t time.Time) int64 { return time.Since(t).Milliseconds() }
