package connectors

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestGunzipRejectsBombOutput(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	junk := bytes.Repeat([]byte("A"), maxCursorInflatedBytes+1024)
	if _, err := zw.Write(junk); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := gunzip(buf.Bytes())
	if err == nil {
		t.Fatalf("expected bomb rejection, got %d bytes", len(out))
	}
	if len(out) > maxCursorInflatedBytes {
		t.Fatalf("output not bounded: %d", len(out))
	}
}

func TestGunzipAcceptsNormalFrame(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte("hello frames")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := gunzip(buf.Bytes())
	if err != nil || string(out) != "hello frames" {
		t.Fatalf("gunzip = %q, %v", out, err)
	}
}

func TestCursorBodyReadIsBounded(t *testing.T) {
	big := bytes.Repeat([]byte("x"), maxCursorBodyBytes+16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(big); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := NewCursor("cursor", srv.URL)
	req := &core.ChatRequest{
		Model:    "cursor-model",
		Messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}}},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected oversize body rejection")
	}
	var perr *core.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("unexpected error type: %v", err)
	}
}
