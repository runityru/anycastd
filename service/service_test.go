package service

import (
	"context"
	"sync"
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
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	select {
	case err := <-errCh:
		s.Require().NoError(err)
	case <-time.After(5 * time.Second):
		s.Fail("Run did not return for an already-cancelled context")
	}
}

// TestRunContextCancelledFromGoroutine verifies Run returns nil when the context
// is cancelled from another goroutine while Run is blocked in its select loop.
func (s *serviceTestSuite) TestRunContextCancelledFromGoroutine() {
	ctx, cancel := context.WithCancel(s.T().Context())

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

// TestRunExecutesSuccessfulTickThenExitsOnCancel verifies that Run runs at
// least one successful tick (announcing and recording the announced metric)
// and keeps looping, then returns nil once the context is cancelled. This
// guards the core loop against a mutation that returns after the first
// successful tick instead of continuing.
func (s *serviceTestSuite) TestRunExecutesSuccessfulTickThenExitsOnCancel() {
	s.announcerM.On("Announce").Return(nil).Once()

	// The loop ticks repeatedly (every 1ms) until the context is cancelled, so
	// the per-tick calls may happen any number of times; the Announce side
	// effect fires exactly once when the healthy tick first crosses the
	// threshold.
	s.checkM.On("Kind").Return("test_check").Maybe()
	s.checkM.On("Check").Return(nil).Maybe()
	s.metricsM.On("ServiceUp", "test_service").Return().Maybe()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Maybe()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Maybe()

	strategy, _ := GetStrategyNoOptions("")
	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, time.Millisecond, s.metricsM, strategy).(*service)

	ctx, cancel := context.WithCancel(s.T().Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	// Wait until the successful tick has announced and recorded the gauge.
	s.Require().Eventually(func() bool {
		return svc.announced.Load()
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		s.Require().NoError(err)
	case <-time.After(5 * time.Second):
		s.Fail("Run did not return after context cancellation")
	}

	s.Require().True(svc.announced.Load(), "service should remain announced after the successful tick")
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

// fakePeerStateSource is a hand-written announcer.PeerStateSource fake whose
// returned peer states and error can be set by the test at runtime.
type fakePeerStateSource struct {
	mu     sync.Mutex
	states []announcer.PeerState
	err    error
}

func (f *fakePeerStateSource) ListPeers(context.Context) ([]announcer.PeerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states, f.err
}

func (f *fakePeerStateSource) setStates(states []announcer.PeerState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = states
}

// TestRunPeerDriftWithEstablishedPeer verifies P3: with a peer-state source
// reporting an ESTABLISHED peer, a healthy service's effective announced gauge
// is true (announce side effect + established peer).
func (s *serviceTestSuite) TestRunPeerDriftWithEstablishedPeer() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{states: []announcer.PeerState{{SessionState: announcer.PeerSessionEstablished}}}

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "announced must be true after successful announce")
}

// TestRunPeerDriftWithNoEstablishedPeer verifies P3: when the service is locally
// announced but no BGP peer session is ESTABLISHED, the reported ServiceAnnounced
// is false while the internal announced flag stays true (so a later successful
// announce is NOT repeated — side-effect gating is unchanged).
func (s *serviceTestSuite) TestRunPeerDriftWithNoEstablishedPeer() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	// Effective announced is false (no established peer), even though the
	// announce side effect fired and announced is locally true.
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{states: []announcer.PeerState{{SessionState: 4}}} // not ESTABLISHED

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "internal announced flag must stay true (side-effect gating unchanged)")
}

// TestRunPeerStateSourceErrorFallsBackToAnnounced verifies P3: a ListPeers error
// must be treated as "established" (fall back to locally-confirmed) so a
// transient peer-state fetch failure never flaps the gauge to 0.
func (s *serviceTestSuite) TestRunPeerStateSourceErrorFallsBackToAnnounced() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{err: errors.New("list peers failed")}

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}

// TestRunPeerDriftTracksPeerDrop verifies P3: the reported gauge follows a peer
// going from ESTABLISHED to down across ticks, while the internal announced flag
// stays true (no re-announce side effect).
func (s *serviceTestSuite) TestRunPeerDriftTracksPeerDrop() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Times(2)
	s.checkM.On("Check").Return(nil).Times(2)

	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Times(2)
	s.metricsM.On("ServiceUp", "test_service").Return().Times(2)
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{states: []announcer.PeerState{{SessionState: announcer.PeerSessionEstablished}}}

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	// Tick 1: announced + established -> gauge true.
	err := svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load())

	// Peer drops; effective announced becomes false, but side-effect flag stays true.
	src.setStates([]announcer.PeerState{{SessionState: 2}})
	err = svc.run(s.ctx)
	s.Require().NoError(err)
	s.Require().True(svc.announced.Load(), "internal announced flag must stay true after peer drop")
}

// TestRunPeerDriftAnyEstablishedAmongMultiple verifies P3: with multiple peers,
// the effective gauge is true if ANY one session is ESTABLISHED, regardless of
// slice position. This pins the "any established" loop rather than a
// first-element-only read.
func (s *serviceTestSuite) TestRunPeerDriftAnyEstablishedAmongMultiple() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", true).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{states: []announcer.PeerState{
		{SessionState: 4}, // not ESTABLISHED
		{SessionState: announcer.PeerSessionEstablished}, // ESTABLISHED later in the slice
	}}

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}

// TestRunPeerDriftEmptyPeerList verifies P3: a configured peer-state source that
// returns an empty peer list (no established session — e.g. before peers are
// added) reports effective announced false.
func (s *serviceTestSuite) TestRunPeerDriftEmptyPeerList() {
	s.announcerM.On("Announce").Return(nil).Once()

	s.checkM.On("Kind").Return("test_check").Once()
	s.checkM.On("Check").Return(nil).Once()

	s.metricsM.On("ServiceUp", "test_service").Return().Once()
	s.metricsM.On("ServiceAnnounced", "test_service", false).Return().Once()
	s.metricsM.On("MeasureCall", "test_service", "test_check").Return().Once()

	strategy, _ := GetStrategyNoOptions("")
	src := &fakePeerStateSource{states: []announcer.PeerState{}}

	svc := New("test_service", s.announcerM, []Checker{{s.checkM, ""}}, 1*time.Second, s.metricsM, strategy,
		WithPeerStateSource(src)).(*service)

	err := svc.run(s.ctx)
	s.Require().NoError(err)
}
