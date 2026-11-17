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
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}

func (s *serviceTestSuite) TestRunFail() {
	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(errors.New("error")).Once()

	s.metricsM.On("ServiceDown", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

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
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Times(2)
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

	for i := 0; i < 3; i++ {
		err := svc.run(s.ctx)
		s.Require().NoError(err)
	}
}

// TestRunAnnounceFailureRetried verifies that a failed Announce does not mark
// the service as announced, so the announce is retried on the next run.
func (s *serviceTestSuite) TestRunAnnounceFailureRetried() {
	aCall1 := s.announcerM.On("Announce").Return(errors.New("announce error")).Once()
	s.announcerM.On("Announce").Return(nil).NotBefore(aCall1).Once()

	s.checkM.On("Kind").Return("test_check").Times(2)
	s.checkM.On("Check").Return(nil).Times(2)

	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Times(2)
	s.metricsM.On("ServiceUp", "test_service").Return().Times(2)
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

	// Run 1: service up, announce fails -> announced must stay false (retried later).
	err := svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().False(svc.announced.Load(), "announced must remain false after failed announce")

	// Run 2: service up, announce is retried and succeeds -> announced becomes true.
	err = svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "announced must become true after successful announce")
}

// TestRunDenounceFailureRetried verifies that a failed Denounce keeps the
// service marked as announced, so the denounce is retried on the next run.
func (s *serviceTestSuite) TestRunDenounceFailureRetried() {
	aCall1 := s.announcerM.On("Announce").Return(nil).Once()
	aCall2 := s.announcerM.On("Denounce").Return(errors.New("denounce error")).NotBefore(aCall1).Once()
	s.announcerM.On("Denounce").Return(nil).NotBefore(aCall2).Once()

	// Run 1: service up, announce succeeds.
	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()
	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Times(2)
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()

	// Run 2: service down, denounce fails -> announced must stay true.
	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(errors.New("check error")).Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()
	s.metricsM.On("ServiceDown", "test_service").Return().Once()

	// Run 3: service down, denounce is retried and succeeds.
	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(errors.New("check error")).Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()
	s.metricsM.On("ServiceDown", "test_service").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

	// Run 1: service up, announce succeeds -> announced becomes true.
	err := svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "announced must be true after successful announce")

	// Run 2: service down, denounce fails -> announced must stay true (retried later).
	err = svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "announced must stay true after failed denounce")

	// Run 3: service down, denounce is retried and succeeds -> announced becomes false.
	err = svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().False(svc.announced.Load(), "announced must become false after successful denounce")
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

// errStrategySentinel is a sentinel returned by injected strategies to verify
// that Run wraps strategy errors and that no side effects fire on strategy
// failure.
var errStrategySentinel = errors.New("strategy sentinel error")

// TestRunContextCancelledPreCancel verifies Run returns nil immediately when the
// context is already cancelled (ctx.Done selected before any run tick).
func (s *serviceTestSuite) TestRunContextCancelledPreCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy)

	err := svc.Run(ctx)
	s.Require().NoError(err)
}

// TestRunContextCancelledFromGoroutine verifies Run returns nil when the context
// is cancelled from another goroutine while Run is blocked in its select loop.
func (s *serviceTestSuite) TestRunContextCancelledFromGoroutine() {
	ctx, cancel := context.WithCancel(context.Background())

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, time.Hour, s.metricsM, strategy)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	cancel()

	select {
	case err := <-errCh:
		s.Require().NoError(err)
	case <-time.After(5 * time.Second):
		s.Fail("Run did not return after context cancellation")
	}
}

// TestRunStrategyError verifies that a strategy error surfaced through run is
// wrapped by Run with a "service error" prefix, preserving the sentinel via the
// error chain (errors.Cause / errors.Is). The context is NOT cancelled here, so
// the returned error can only originate from the strategy.
func (s *serviceTestSuite) TestRunStrategyError() {
	strategy := func([]CheckResult) (bool, error) {
		return false, errStrategySentinel
	}

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, time.Nanosecond, s.metricsM, strategy)

	err := svc.Run(s.ctx)
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "service error")
	s.Require().True(errors.Is(err, errStrategySentinel), "wrapped error must preserve the sentinel via Is")
	s.Require().Equal(errStrategySentinel, errors.Cause(err), "errors.Cause must unwrap to the sentinel")
}

// TestRunStrategyErrorNoSideEffects verifies that when the strategy returns an
// error, run aborts before any side effect: no ServiceUp/ServiceDown/Announced
// metrics and no Announce/Denounce announcer calls.
func (s *serviceTestSuite) TestRunStrategyErrorNoSideEffects() {
	strategy := func([]CheckResult) (bool, error) {
		return false, errStrategySentinel
	}

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy).(*service)

	err := svc.run(s.ctx)
	s.Require().Error(err)

	s.announcerM.AssertNotCalled(s.T(), "Announce")
	s.announcerM.AssertNotCalled(s.T(), "Denounce")
	s.metricsM.AssertNotCalled(s.T(), "ServiceUp")
	s.metricsM.AssertNotCalled(s.T(), "ServiceDown")
	s.metricsM.AssertNotCalled(s.T(), "ServiceAnnounced")
}
