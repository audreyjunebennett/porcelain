package control

import (
	"strings"
	"sync"
)

// State tracks wrapper readiness exposed on the supervisor control API.
type State struct {
	mu                  sync.RWMutex
	brokerRequired      bool
	vectorstoreRequired bool
	embedRequired       bool
	brokerReady         bool
	vectorstoreReady    bool
	embedReady          bool
	brokerRestarts      int
	vectorstoreRestarts int
	embedRestarts       int
	lastError           string
	wrapperVersion      string
	buildCommit         string
	brokerEndpoint      string
	vectorstoreEndpoint string
	embedEndpoint       string
	operatorUIBaseURL   string
	bootstrap           bool
}

// NewState returns an empty control-plane state.
func NewState() *State {
	return &State{}
}

func (s *State) SetRequired(brokerRequired, vectorstoreRequired bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerRequired = brokerRequired
	s.vectorstoreRequired = vectorstoreRequired
}

func (s *State) SetEmbedRequired(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedRequired = v
}

func (s *State) SetVersions(wrapperVersion, buildCommit string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wrapperVersion = strings.TrimSpace(wrapperVersion)
	s.buildCommit = strings.TrimSpace(buildCommit)
}

func (s *State) SetEndpoints(brokerEndpoint, vectorstoreEndpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerEndpoint = strings.TrimSpace(brokerEndpoint)
	s.vectorstoreEndpoint = strings.TrimSpace(vectorstoreEndpoint)
}

func (s *State) SetEmbedEndpoint(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedEndpoint = strings.TrimSpace(v)
}

func (s *State) SetBrokerReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerReady = v
}

func (s *State) SetVectorstoreReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorstoreReady = v
}

func (s *State) SetEmbedReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedReady = v
}

func (s *State) IncBrokerRestarts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerRestarts++
}

func (s *State) IncVectorstoreRestarts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorstoreRestarts++
}

func (s *State) SetLastError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = strings.TrimSpace(err)
}

func (s *State) SetOperatorUI(baseURL string, bootstrap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operatorUIBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.bootstrap = bootstrap
}

// Snapshot is a point-in-time copy of control-plane state.
type Snapshot struct {
	BrokerRequired      bool
	VectorstoreRequired bool
	EmbedRequired       bool
	BrokerReady         bool
	VectorstoreReady    bool
	EmbedReady          bool
	BrokerRestarts      int
	VectorstoreRestarts int
	EmbedRestarts       int
	LastError           string
	WrapperVersion      string
	BuildCommit         string
	BrokerEndpoint      string
	VectorstoreEndpoint string
	EmbedEndpoint       string
	OperatorUIBaseURL   string
	Bootstrap           bool
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		BrokerRequired:      s.brokerRequired,
		VectorstoreRequired: s.vectorstoreRequired,
		EmbedRequired:       s.embedRequired,
		BrokerReady:         s.brokerReady,
		VectorstoreReady:    s.vectorstoreReady,
		EmbedReady:          s.embedReady,
		BrokerRestarts:      s.brokerRestarts,
		VectorstoreRestarts: s.vectorstoreRestarts,
		EmbedRestarts:       s.embedRestarts,
		LastError:           s.lastError,
		WrapperVersion:      s.wrapperVersion,
		BuildCommit:         s.buildCommit,
		BrokerEndpoint:      s.brokerEndpoint,
		VectorstoreEndpoint: s.vectorstoreEndpoint,
		EmbedEndpoint:       s.embedEndpoint,
		OperatorUIBaseURL:   s.operatorUIBaseURL,
		Bootstrap:           s.bootstrap,
	}
}

// ContractStatus returns "ok" or "degraded" for required children.
func ContractStatus(s Snapshot) string {
	if s.BrokerRequired && !s.BrokerReady {
		return "degraded"
	}
	if s.VectorstoreRequired && !s.VectorstoreReady {
		return "degraded"
	}
	if s.EmbedRequired && !s.EmbedReady {
		return "degraded"
	}
	return "ok"
}

// Ready reports whether all required children are ready.
func Ready(s Snapshot) bool {
	return ContractStatus(s) == "ok"
}
