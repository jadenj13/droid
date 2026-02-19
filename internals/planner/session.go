package planner

import (
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jadenj13/droid/internals/git"
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

type Message struct {
	Role      string // "user", "assistant", or "tool_result"
	Content   string // plain text, or JSON-serialised content blocks for assistant tool calls
	RawBlocks []anthropic.ToolResultBlockParam // populated for tool_result role only
}

type Session struct {
	ThreadTS  string
	ChannelID string
	Stage     Stage
	Messages  []Message

	Repo    *git.RepoInfo
	Tracker git.Tracker

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
		Messages:  []Message{},
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
	sess.Messages = append(sess.Messages, Message{Role: role, Content: content})
	return s.Save(sess)
}
