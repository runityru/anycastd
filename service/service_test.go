package service

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/runityru/anycastd/announcer"
	"github.com/runityru/anycastd/checkers"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func (s *serviceTestSuite) TestRunPass() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	svc := New("test_service", s.announcerM, []checkers.Checker{s.checkM}, 1*time.Second, s.metricsM).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}

func (s *serviceTestSuite) TestRunFail() {
	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(errors.New("error")).Once()

	s.metricsM.On("ServiceDown", "test_service").Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	svc := New("test_service", s.announcerM, []checkers.Checker{s.checkM}, 1*time.Second, s.metricsM).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}

func (s *serviceTestSuite) TestRunPassThenFailThenPass() {
	aCall1 := s.announcerM.On("Announce").Return(nil).Once()
	aCall2 := s.announcerM.On("Denounce").Return(nil).NotBefore(aCall1).Once()
	s.announcerM.On("Announce").Return(nil).NotBefore(aCall2).Once()

	s.checkM.On("Kind").Return("test_check").Times(3)
	cCall1 := s.checkM.On("Check").Return(nil).Once()
	cCall2 := s.checkM.On("Check").Return(errors.New("error")).NotBefore(cCall1).Once()
	s.checkM.On("Check").Return(nil).NotBefore(cCall2).Once()

	mCall1 := s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()
	mCall2 := s.metricsM.On("ServiceUp", "test_service").Return().NotBefore(mCall1).Once()
	mCall3 := s.metricsM.On("MeasureCall", "test_service", "test_check").Return().NotBefore(mCall2).Once()
	mCall4 := s.metricsM.On("ServiceDown", "test_service").Return().NotBefore(mCall3).Once()
	mCall5 := s.metricsM.On("MeasureCall", "test_service", "test_check").Return().NotBefore(mCall4).Once()
	s.metricsM.On("ServiceUp", "test_service").Return().NotBefore(mCall5).Once()

	svc := New("test_service", s.announcerM, []checkers.Checker{s.checkM}, 1*time.Second, s.metricsM).(*service)

	for i := 0; i < 3; i++ {
		err := svc.run(s.ctx)
		s.Require().NoError(err)
	}
}

// Definitions ...
type serviceTestSuite struct {
	suite.Suite

	ctx        context.Context
	announcerM *announcer.Mock
	checkM     *checkers.Mock
	metricsM   *MetricsMock
}

func (s *serviceTestSuite) SetupTest() {
	s.ctx = context.Background()

	s.announcerM = announcer.NewMock()
	s.checkM = checkers.NewMock()
	s.metricsM = NewMetricsMock()
}

func (s *serviceTestSuite) TearDownTest() {
	s.announcerM.AssertExpectations(s.T())
	s.checkM.AssertExpectations(s.T())
	s.metricsM.AssertExpectations(s.T())
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, &serviceTestSuite{})
}
