module github.com/jaywehosl/quic-diver

go 1.26.0

require (
	github.com/cilium/ebpf v0.22.0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/quic-go/connect-ip-go v0.1.0
	github.com/quic-go/quic-go v0.60.0
	github.com/yosida95/uritemplate/v3 v3.0.2
	golang.org/x/sys v0.47.0
	gvisor.dev/gvisor v0.0.0-20231104011432-48a6d7d5bd0b
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dunglas/httpsfv v1.0.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/mobile v0.0.0-20260821190718-4776eadac327 // indirect
	golang.org/x/mod v0.39.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/quic-go/quic-go => ./third_party/quic-go

replace github.com/quic-go/connect-ip-go => ./third_party/connect-ip-go
