package pace

import "time"

const (
	Stagger       = 250 * time.Millisecond
	QUICHeadStart = time.Second
	RelayStart    = 2500 * time.Millisecond
	LateFor       = 6 * time.Second
	DialWait      = 20 * time.Second
	ControlWait   = 8 * time.Second

	ProbeEvery = 2 * time.Minute
	ProbeWait  = 4 * time.Second
	SwitchRest = 90 * time.Second

	WatchStep  = 3 * time.Second
	Deaf       = 20 * time.Second
	Patience   = 45 * time.Second
	Silence    = 60 * time.Second
	AskWait    = 3 * time.Second
	GlanceWait = 700 * time.Millisecond
	ProveWait  = 1500 * time.Millisecond
	Frozen     = 30 * time.Second
	Gone       = 75 * time.Second

	MoveWait  = 1500 * time.Millisecond
	MovePause = time.Second
	MoveTries = 1
)

func Backoff(was time.Duration) time.Duration {
	switch {
	case was < 3*time.Second:
		return 3 * time.Second
	case was < 5*time.Second:
		return 5 * time.Second
	case was < 10*time.Second:
		return 10 * time.Second
	case was < 20*time.Second:
		return 20 * time.Second
	default:
		return 30 * time.Second
	}
}
