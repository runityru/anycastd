package assigned_address

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ptr "github.com/teran/go-ptr"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestSpec(t *testing.T) {
	r := require.New(t)

	data, err := os.ReadFile("testdata/spec.json")
	r.NoError(err)

	c, err := NewFromSpec(json.RawMessage(data))
	r.NoError(err)

	aa := c.(*assigned_address)
	r.Equal("127.0.0.33", aa.ipv4)
	r.Equal("dummy0", *aa.iface)
}

func TestCheckHappyPath(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4:      "127.0.0.232",
		Interface: ptr.String("test0"),
	})
	r.NoError(err)

	c.(*assigned_address).interfaceCollector = func() (map[string]string, error) {
		return map[string]string{
			"127.0.0.232": "test0",
		}, nil
	}

	err = c.Check(context.Background())
	r.NoError(err)
}

func TestCheckNotMatchedInterface(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4:      "127.0.0.232",
		Interface: ptr.String("blah0"),
	})
	r.NoError(err)

	c.(*assigned_address).interfaceCollector = func() (map[string]string, error) {
		return map[string]string{
			"127.0.0.232": "test0",
		}, nil
	}

	err = c.Check(context.Background())
	r.Error(err)
	r.Equal("Interface name is not matched for described IPv4 address", err.Error())
}

func TestCheckEmptyInterface(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4: "127.0.0.232",
	})
	r.NoError(err)

	c.(*assigned_address).interfaceCollector = func() (map[string]string, error) {
		return map[string]string{
			"127.0.0.232": "test0",
		}, nil
	}

	err = c.Check(context.Background())
	r.NoError(err)
}

func TestCheckNotFoundIPv4Address(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4: "123.45.67.89",
	})
	r.NoError(err)

	c.(*assigned_address).interfaceCollector = func() (map[string]string, error) {
		return map[string]string{
			"127.0.0.232": "test0",
		}, nil
	}

	err = c.Check(context.Background())
	r.Error(err)
	r.Equal("no IPv4 address found on the system", err.Error())
}
