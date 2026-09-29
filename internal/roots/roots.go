package roots

import (
	"crypto/x509"
	_ "embed"
	"sync"
)

//go:embed isrg.pem
var isrg []byte

var Pool = sync.OnceValue(func() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(isrg)
	return pool
})
