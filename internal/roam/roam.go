// Package roam следит за сменой сети на машине клиента.
package roam

// Watcher сообщает о смене сети: адрес поменялся, маршрут переехал, интерфейс
// поднялся или лёг.
type Watcher interface {
	Changed() <-chan struct{}
	Close() error
}
