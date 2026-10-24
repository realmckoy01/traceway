package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitLabSend(t *testing.T) {
	for _, project := range []string{"123", "group/subgroup/project"} {
		t.Run(project, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.RequestURI != "/gitlab/api/v4/projects/"+strings.ReplaceAll(project, "/", "%2F")+"/issues" {
					t.Errorf("unexpected request: %s %s", r.Method, r.RequestURI)
				}
				if r.Header.Get("PRIVATE-TOKEN") != "test-token" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing headers")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["title"] != "Error" || body["description"] != "Details" || body["labels"] != "bug,traceway" {
					t.Errorf("unexpected payload: %#v", body)
				}
				w.WriteHeader(201)
			}))
			defer server.Close()
			config, _ := json.Marshal(GitLabAdapter{BaseURL: server.URL + "/gitlab/", Token: "test-token", ProjectID: project, Labels: []string{"bug", "traceway"}})
			adapter, err := NewAdapter("gitlab", config)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.Send(context.Background(), Message{Subject: "Error", Body: "Details"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGitLabValidate(t *testing.T) {
	a := GitLabAdapter{Token: "token", ProjectID: "123"}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	u, _ := a.instanceURL()
	if u.String() != "https://gitlab.com" {
		t.Fatal("wrong default")
	}
	for _, base := range []string{"gitlab.example", "ftp://gitlab.example", "https://user:pass@gitlab.example", "https://gitlab.example?q=1", "https://gitlab.example#fragment"} {
		a.BaseURL = base
		if err := a.Validate(); err == nil {
			t.Errorf("accepted %q", base)
		}
	}
	for _, cfg := range []GitLabAdapter{{Token: " ", ProjectID: "123"}, {Token: "token", ProjectID: " "}} {
		if err := cfg.Validate(); err == nil {
			t.Error("accepted missing credentials or project")
		}
	}
	if _, err := NewAdapter("gitlab", json.RawMessage(`{"token":`)); err == nil {
		t.Error("accepted invalid JSON")
	}
}

func TestGitLabFailures(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("sensitive response"))
			}))
			defer server.Close()
			a := GitLabAdapter{BaseURL: server.URL, Token: "token", ProjectID: "123"}
			err := a.Send(context.Background(), Message{})
			if err == nil || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := GitLabAdapter{Token: "token", ProjectID: "123"}
	if err := a.Send(ctx, Message{}); err == nil {
		t.Error("ignored cancellation")
	}
}
