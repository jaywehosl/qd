// Package packet описывает источник и сток сырых IP-пакетов.
//
// Один контракт (Source) поверх любого механизма захвата: WinDivert на Windows,
// TUN на Linux/Android/macOS/iOS. Ядро выше этого слоя не знает, откуда пришёл
// пакет. Recv/Send — батчевые: батч критичен для производительности и ровного
// пинга (arch6), т.к. амортизирует syscall'ы захвата/инжекта.
package packet

import "context"

// Direction — направление пакета относительно локального хоста.
type Direction uint8

const (
	// Outbound — пакет идёт из хоста в сеть (кандидат на проксирование).
	Outbound Direction = iota
	// Inbound — пакет идёт из сети в хост (ответ туннеля, реинжект).
	Inbound
)

// Packet — один сырой IP-пакет (IPv4 или IPv6) плюс метаданные захвата.
//
// Data содержит IP-заголовок и payload целиком. Владение слайсом — до
// следующего вызова Recv у того же Source (реализация вправе переиспользовать
// буфер батча). Кто держит данные дольше — копирует.
type Packet struct {
	Data []byte
	Dir  Direction
	// PID процесса-источника; 0, если неизвестен. WinDivert отдаёт его через
	// FLOW/SOCKET-слой; TUN обычно нет — тогда per-process недоступен.
	PID uint32
	// IfIndex — индекс сетевого интерфейса захвата.
	IfIndex uint32
}

// Source — двусторонний канал сырых IP-пакетов.
type Source interface {
	// Recv блокируется до появления пакетов и возвращает батч.
	// Возвращённые Packet.Data валидны до следующего Recv.
	Recv(ctx context.Context) ([]Packet, error)

	// Send инжектит батч обратно (в сеть для Inbound-ответов или в стек хоста).
	Send(pkts []Packet) error

	// MTU пути захвата в байтах.
	MTU() int

	// Close освобождает драйвер/устройство захвата.
	Close() error
}

// MultiSource — источник, допускающий параллельный приём/инжект несколькими
// горутинами. Один поток захвата упирается в потолок задолго до полосы канала
// (замерено: тракт с одним читателем/писателем режет ~700 Мбит до ~300).
//
// Каждый Reader/Writer держит СВОИ буферы, поэтому работают независимо.
type MultiSource interface {
	Source
	NewReader() Reader
	NewWriter() Writer
}

// Reader — независимый приёмник пакетов.
type Reader interface {
	// Recv возвращает батч; Packet.Data валидны до следующего Recv этого Reader.
	Recv(ctx context.Context) ([]Packet, error)
}

// Writer — независимый инжектор пакетов.
type Writer interface {
	Send(pkts []Packet) error
}
