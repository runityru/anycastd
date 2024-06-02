package service

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/teran/anycastd/announcer"
	"github.com/teran/anycastd/checkers"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestRunPass(t *testing.T) {
	r := require.New(t)

	announcerM := announcer.NewMock()
	defer announcerM.AssertExpectations(t)

	announcerM.On("Announce").Return(nil).Once()

	checkM := checkers.NewMock()
	defer checkM.AssertExpectations(t)

	checkM.On("Check").Return(nil).Once()

	svc := New(announcerM, []checkers.Checker{checkM}, 1*time.Second).(*service)

	err := svc.run(context.TODO())
	r.NoError(err)
}

func TestRunFail(t *testing.T) {
	r := require.New(t)

	announcerM := announcer.NewMock()
	defer announcerM.AssertExpectations(t)

	announcerM.On("Announce").Return(nil).Once()

	checkM := checkers.NewMock()
	defer checkM.AssertExpectations(t)

	checkM.On("Check").Return(errors.New("error")).Once()

	svc := New(announcerM, []checkers.Checker{checkM}, 1*time.Second).(*service)

	err := svc.run(context.TODO())
	r.NoError(err)
}
