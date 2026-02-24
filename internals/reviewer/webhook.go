package reviewer

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
)

type WebhookServer struct {
	worker       *Worker
	githubSecret string
	gitlabSecret string
	log          *slog.Logger
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

type githubPRPayload struct {
	Action string `json:"action"`
	Label  struct {
		Name string `json:"name"`
	} `json:"label"`
	PullRequest struct {
		Number int    `json:"number"`
		URL    string `json:"html_url"`
	} `json:"pull_request"`
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

	if r.Header.Get("x-github-event") != "pull_request" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var payload githubPRPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if payload.Action != "labeled" || payload.Label.Name != "agent:review" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	prNumber := payload.PullRequest.Number
	repoURL := payload.Repository.HTMLURL

	go func() {
		ctx := context.Background()
		if err := s.worker.HandlePR(ctx, repoURL, prNumber); err != nil {
			s.log.Error("reviewer failed", "pr", prNumber, "err", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

type gitlabMRPayload struct {
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
		IID int `json:"iid"`
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

	var payload gitlabMRPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if payload.ObjectKind != "merge_request" || !labelAdded(payload.Changes.Labels.Current, payload.Changes.Labels.Previous, "agent:review") {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	mrNumber := payload.ObjectAttributes.IID
	repoURL := payload.Project.WebURL

	go func() {
		ctx := context.Background()
		if err := s.worker.HandlePR(ctx, repoURL, mrNumber); err != nil {
			s.log.Error("reviewer failed", "mr", mrNumber, "err", err)
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
		return body, nil
	}
	sig := strings.TrimPrefix(r.Header.Get(sigHeader), "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig)) {
		return nil, fmt.Errorf("signature mismatch")
	}
	return body, nil
}

func labelAdded(current, previous []struct {
	Name string `json:"name"`
}, label string) bool {
	for _, l := range previous {
		if l.Name == label {
			return false
		}
	}
	for _, l := range current {
		if l.Name == label {
			return true
		}
	}
	return false
}
