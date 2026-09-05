package roam

type Watcher interface {
	Changed() <-chan struct{}
	Close() error
}
