package announcer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnouncer(t *testing.T) {
	r := require.New(t)

	goBgpM := newGoBGPMock()

	call1 := goBgpM.On(
		"AddPath",
		"127.0.0.1/32",
		"127.0.0.2",
	).Return(nil).Once()
	goBgpM.On(
		"DeletePath",
		"127.0.0.1/32",
	).Return(nil).NotBefore(call1).Once()

	a := New(Config{
		GoBGP:    goBgpM,
		Prefixes: []string{"127.0.0.1/32"},
		NextHop:  "127.0.0.2",
	})

	err := a.Announce(context.Background())
	r.NoError(err)

	err = a.Denounce(context.Background())
	r.NoError(err)
}
