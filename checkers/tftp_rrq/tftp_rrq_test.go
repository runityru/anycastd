package tftp_rrq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pin/tftp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/teran/go-ptr"
	th "github.com/teran/go-time"
)

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	p := c.(*tftp_rrq)
	r.Equal("tftp://127.0.0.1:69/lpxelinux.0", p.url)
	r.Equal("09da9c01b6b2a8ccc5d3445c4f364243d8a063bd0bf520643737899e6ce0170f", *p.expSHA256)
	r.Equal(uint8(3), p.tries)
	r.Equal(100*time.Millisecond, p.interval)
	r.Equal(5*time.Second, p.timeout)
}

func TestCheck(t *testing.T) {
	type testCase struct {
		name     string
		in       func(address string) spec
		expError error
	}

	const mockChecksum = "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"

	tcs := []testCase{
		{
			name: "valid configuration w/o checksum",
			in: func(address string) spec {
				return spec{
					URL:      fmt.Sprintf("tftp://%s/test", address),
					Tries:    3,
					Interval: th.Duration(100 * time.Millisecond),
					Timeout:  th.Duration(1 * time.Second),
				}
			},
		},
		{
			name: "valid configuration w/ checksum",
			in: func(address string) spec {
				return spec{
					URL:            fmt.Sprintf("tftp://%s/test", address),
					ExpectedSHA256: ptr.String(mockChecksum),
					Tries:          3,
					Interval:       th.Duration(100 * time.Millisecond),
					Timeout:        th.Duration(1 * time.Second),
				}
			},
		},
		{
			name: "valid configuration w/ invalid checksum",
			in: func(address string) spec {
				return spec{
					URL:            fmt.Sprintf("tftp://%s/test", address),
					ExpectedSHA256: ptr.String(strings.ReplaceAll(mockChecksum, "a", "b")),
					Tries:          3,
					Interval:       th.Duration(100 * time.Millisecond),
					Timeout:        th.Duration(1 * time.Second),
				}
			},
			expError: errors.Errorf(
				"check failed: 3 tries with 100ms interval; last error: `checksum mismatch: expected `%s` != actual `%s``",
				strings.ReplaceAll(mockChecksum, "a", "b"), mockChecksum,
			),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			l, err := net.ListenUDP("udp", &net.UDPAddr{
				IP:   net.IPv4(127, 0, 0, 1),
				Port: 0,
				Zone: "",
			})
			r.NoError(err)

			defer func() { _ = l.Close() }()

			s := tftp.NewServer(func(filename string, rf io.ReaderFrom) error {
				buf := bytes.NewBuffer([]byte("test data"))
				_, err := rf.ReadFrom(buf)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					return err
				}
				return nil
			}, nil)
			defer s.Shutdown()

			s.SetTimeout(5 * time.Second)

			go func(srv *tftp.Server, l *net.UDPConn) {
				s.Serve(l)
			}(s, l)

			c, err := New(tc.in(l.LocalAddr().String()))
			r.NoError(err)

			err = c.Check(context.Background())
			if tc.expError != nil {
				r.Error(err)
				r.Equal(tc.expError.Error(), err.Error())
			} else {
				r.NoError(err)
			}
		})
	}
}
