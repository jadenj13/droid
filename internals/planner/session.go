package planner

import (
	"sync"
	"time"

	"github.com/jadenj13/droid/internals/git"
	"github.com/jadenj13/droid/internals/llm"
)

type Stage int

const (
	StageBrainstorm Stage = iota
	StagePRD
	StageCriteria
	StageIssues
	StageDone
)

func (s Stage) String() string {
	return [...]string{"brainstorm", "prd", "criteria", "issues", "done"}[s]
}

type Session struct {
	ThreadTS  string
	ChannelID string
	Stage     Stage
	Messages  []llm.Message

	Repo        *git.RepoInfo
	GitProvider git.GitProvider

	PRDDraft string
	Criteria []string
	Issues   []LinkedIssue

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LinkedIssue struct {
	Number int
	Title  string
	URL    string
}

func newSession(threadTS, channelID string) *Session {
	return &Session{
		ThreadTS:  threadTS,
		ChannelID: channelID,
		Stage:     StageBrainstorm,
		Messages:  []llm.Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session // key: threadTS
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

func (s *SessionStore) GetOrCreate(threadTS, channelID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[threadTS]; ok {
		return sess
	}

	sess := newSession(threadTS, channelID)
	s.sessions[threadTS] = sess
	return sess
}

func (s *SessionStore) Get(threadTS string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[threadTS]
	return sess, ok
}

func (s *SessionStore) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.UpdatedAt = time.Now()
	s.sessions[sess.ThreadTS] = sess
	return nil
}

func (s *SessionStore) AppendMessage(sess *Session, role, content string) error {
	sess.Messages = append(sess.Messages, llm.Message{Role: role, Content: content})
	return s.Save(sess)
}
