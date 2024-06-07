package announcer

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var reProtoString = regexp.MustCompile(
	`^\[type\.googleapis\.com\/apipb\.IPAddressPrefix\]:\{prefix_len:32\s{1,2}prefix:\"172.16.38.43\"\}$`,
)

func TestAnnouncer(t *testing.T) {
	r := require.New(t)

	goBgpM := newGoBGPMock()

	matcher := func(in string) bool {
		return reProtoString.MatchString(in)
	}

	call1 := goBgpM.On(
		"AddPath",
		mock.MatchedBy(matcher),
	).Return([]byte("123456"), nil).Once()
	goBgpM.On(
		"DeletePath",
		mock.MatchedBy(matcher),
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
