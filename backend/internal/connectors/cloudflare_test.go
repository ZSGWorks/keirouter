package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestCloudflareModelSourceUsesGetSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/accounts/a/ai/v1/models/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"@cf/example/model","name":"Example"}]}`))
	}))
	defer server.Close()

	source := &CloudflareModelSource{defaultBase: server.URL + "/accounts/{accountId}/ai/v1"}
	models, err := source.ListModels(context.Background(), core.Credentials{AccessToken: "token", Extra: map[string]string{"accountId": "a"}})
	if err != nil || len(models) != 1 || models[0].ID != "@cf/example/model" {
		t.Fatalf("models = %#v, err = %v", models, err)
	}
}
