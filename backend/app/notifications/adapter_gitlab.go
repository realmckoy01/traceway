package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GitLabAdapter struct {
	BaseURL   string   `json:"baseUrl,omitempty"`
	Token     string   `json:"token"`
	ProjectID string   `json:"projectId"`
	Labels    []string `json:"labels,omitempty"`
}

func (a *GitLabAdapter) Type() string { return "gitlab" }

func (a *GitLabAdapter) Validate() error {
	if strings.TrimSpace(a.Token) == "" {
		return fmt.Errorf("GitLab token is required")
	}
	if strings.TrimSpace(a.ProjectID) == "" {
		return fmt.Errorf("GitLab project ID or path is required")
	}
	_, err := a.instanceURL()
	return err
}

func (a *GitLabAdapter) instanceURL() (*url.URL, error) {
	raw := strings.TrimSpace(a.BaseURL)
	if raw == "" {
		raw = "https://gitlab.com"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("GitLab instance URL must be an HTTP or HTTPS URL without credentials, query parameters, or a fragment")
	}
	return u, nil
}

func (a *GitLabAdapter) Send(ctx context.Context, msg Message) error {
	if err := a.Validate(); err != nil {
		return err
	}
	instance, _ := a.instanceURL()
	target := strings.TrimRight(instance.String(), "/") + "/api/v4/projects/" + url.PathEscape(strings.TrimSpace(a.ProjectID)) + "/issues"
	payload := map[string]string{"title": msg.Subject, "description": msg.Body}
	if len(a.Labels) > 0 {
		payload["labels"] = strings.Join(a.Labels, ",")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal GitLab payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create GitLab request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", a.Token)
	// Never forward the private token to a redirect destination.
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GitLab request failed")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("GitLab returned status %d", resp.StatusCode)
	}
	return nil
}
