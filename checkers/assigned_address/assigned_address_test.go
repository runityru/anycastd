package assigned_address

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ptr "github.com/teran/go-ptr"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestCheckHappyPath(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4:      "127.0.0.1",
		Interface: ptr.String("lo0"),
	})
	r.NoError(err)

	err = c.Check(context.Background())
	r.NoError(err)
}

func TestCheckNotMatchedInterface(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4:      "127.0.0.1",
		Interface: ptr.String("blah0"),
	})
	r.NoError(err)

	err = c.Check(context.Background())
	r.Error(err)
	r.Equal("Interface name is not matched for described IPv4 address", err.Error())
}

func TestCheckEmptyInterface(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4: "127.0.0.1",
	})
	r.NoError(err)

	err = c.Check(context.Background())
	r.NoError(err)
}

func TestCheckNotFoundIPv4Address(t *testing.T) {
	r := require.New(t)

	c, err := New(spec{
		IPv4: "123.45.67.89",
	})
	r.NoError(err)

	err = c.Check(context.Background())
	r.Error(err)
	r.Equal("no IPv4 address found on the system", err.Error())
}
