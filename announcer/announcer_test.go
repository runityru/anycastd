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
		"[type.googleapis.com/apipb.IPAddressPrefix]:{prefix_len:32  prefix:\"172.16.38.43\"}",
	).Return([]byte("123456"), nil).Once()
	goBgpM.On(
		"DeletePath",
		"[type.googleapis.com/apipb.IPAddressPrefix]:{prefix_len:32  prefix:\"172.16.38.43\"}",
	).Return(nil).NotBefore(call1).Once()

	a := New(Config{
		GoBGP:    goBgpM,
		Prefixes: []string{"172.16.38.43/32"},
		NextHop:  "172.12.33.14",
	})

	err := a.Announce(context.Background())
	r.NoError(err)

	err = a.Denounce(context.Background())
	r.NoError(err)
}
