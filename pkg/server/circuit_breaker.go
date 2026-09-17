package server

import (
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func (s *Server) armWatchdog(memento *runtimeState, duration time.Duration) {
	s.watchdogMu.Lock()
	defer s.watchdogMu.Unlock()

	s.mementoState = memento
	s.watchdogErrors.Store(0)
	s.watchdogActive = true

	time.AfterFunc(duration, func() {
		s.watchdogMu.Lock()
		defer s.watchdogMu.Unlock()
		if s.watchdogActive {
			s.watchdogActive = false
			s.mementoState = nil
		}
	})
}

func (s *Server) recordProxyError() {
	s.watchdogMu.Lock()
	defer s.watchdogMu.Unlock()

	if !s.watchdogActive || s.mementoState == nil {
		return
	}

	if s.watchdogErrors.Add(1) >= 3 {
		s.state.Store(s.mementoState)
		s.watchdogActive = false
		s.mementoState = nil
	}
}

func (s *Server) recordProxySuccess() {
	s.watchdogErrors.Store(0)
}

func (s *Server) allowProvider(p provider.LLMProvider) bool {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		return cbProvider.CircuitBreaker().AllowRequest()
	}
	return true
}

func (s *Server) recordProviderFailure(p provider.LLMProvider) {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		cbProvider.CircuitBreaker().RecordFailure()
	}
}

func (s *Server) recordProviderSuccess(p provider.LLMProvider) {
	if cbProvider, ok := p.(provider.CircuitBreakerProvider); ok {
		cbProvider.CircuitBreaker().RecordSuccess()
	}
}
