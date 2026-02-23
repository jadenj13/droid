package executor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jadenj13/droid/internals/git"
)

type WebhookServer struct {
	worker          *Worker
	githubSecret    string
	gitlabSecret    string
	log             *slog.Logger
}

func NewWebhookServer(worker *Worker, githubSecret, gitlabSecret string, log *slog.Logger) *WebhookServer {
	return &WebhookServer{
		worker:       worker,
		githubSecret: githubSecret,
		gitlabSecret: gitlabSecret,
		log:          log,
	}
}

func (s *WebhookServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", s.handleGitHub)
	mux.HandleFunc("/webhook/gitlab", s.handleGitLab)
	return mux
}

type githubWebhookPayload struct {
	Action string `json:"action"`
	Label  struct {
		Name string `json:"name"`
	} `json:"label"`
	Issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		URL    string `json:"html_url"`
		Body   string `json:"body"`
	} `json:"issue"`
	Repository struct {
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
}

func (s *WebhookServer) handleGitHub(w http.ResponseWriter, r *http.Request) {
	body, err := s.readAndVerify(r, s.githubSecret, "x-hub-signature-256")
	if err != nil {
		s.log.Warn("github webhook verify failed", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("x-github-event")
	if event != "issues" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var payload githubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if payload.Action != "labeled" || payload.Label.Name != "agent:ready" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	issue := git.Issue{
		Number: payload.Issue.Number,
		Title:  payload.Issue.Title,
		URL:    payload.Issue.URL,
	}

	go func() {
		ctx := context.Background()
		if err := s.worker.HandleIssue(ctx, payload.Repository.HTMLURL, issue); err != nil {
			s.log.Error("handle issue failed", "issue", issue.Number, "err", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

type gitlabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Changes    struct {
		Labels struct {
			Current []struct {
				Name string `json:"name"`
			} `json:"current"`
			Previous []struct {
				Name string `json:"name"`
			} `json:"previous"`
		} `json:"labels"`
	} `json:"changes"`
	ObjectAttributes struct {
		IID   int    `json:"iid"`
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"object_attributes"`
	Project struct {
		WebURL string `json:"web_url"`
	} `json:"project"`
}

func (s *WebhookServer) handleGitLab(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-gitlab-token") != s.gitlabSecret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload gitlabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if payload.ObjectKind != "issue" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !labelAdded(payload.Changes.Labels.Current, payload.Changes.Labels.Previous, "agent:ready") {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	issue := git.Issue{
		Number: payload.ObjectAttributes.IID,
		Title:  payload.ObjectAttributes.Title,
		URL:    payload.ObjectAttributes.URL,
	}

	go func() {
		ctx := context.Background()
		if err := s.worker.HandleIssue(ctx, payload.Project.WebURL, issue); err != nil {
			s.log.Error("handle issue failed", "issue", issue.Number, "err", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *WebhookServer) readAndVerify(r *http.Request, secret, sigHeader string) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		return body, nil // verification disabled
	}
	sig := r.Header.Get(sigHeader)
	if !verifyHMAC(body, secret, sig) {
		return nil, fmt.Errorf("signature mismatch")
	}
	return body, nil
}

func labelAdded(current, previous []struct{ Name string `json:"name"` }, label string) bool {
	inPrev := false
	for _, l := range previous {
		if l.Name == label {
			inPrev = true
			break
		}
	}
	if inPrev {
		return false
	}
	for _, l := range current {
		if l.Name == label {
			return true
		}
	}
	return false
}

func verifyHMAC(body []byte, secret, sig string) bool {
	sig = strings.TrimPrefix(sig, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}