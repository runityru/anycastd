package tls_certificate

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func httpRequestHandler(w http.ResponseWriter, req *http.Request) {
	_, _ = w.Write([]byte("Hello,World!\n"))
}

func TestGetRemoteCertificate(t *testing.T) {
	r := require.New(t)

	l := newLocalListener()

	go func() {
		serverTLSCert, err := tls.LoadX509KeyPair(
			"testdata/test_cert_with_ca.pem",
			"testdata/test_cert_with_ca_key.pem",
		)
		if err != nil {
			panic(err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{serverTLSCert},
		}
		server := &http.Server{
			Handler:   http.HandlerFunc(httpRequestHandler),
			TLSConfig: tlsConfig,
		}
		defer func() { _ = server.Close() }()

		if err := server.ServeTLS(l, "", ""); err != nil {
			panic(err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	fn := getRemoteCertificate(Remote{
		Address:  l.Addr().String(),
		Insecure: true,
	})

	crts, err := fn()
	r.NoError(err)
	r.Len(crts, 2)
	r.Equal("Test certificate", crts[0].Subject.CommonName)
	r.Equal("Test CA", crts[1].Subject.CommonName)
}

func newLocalListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l
}
