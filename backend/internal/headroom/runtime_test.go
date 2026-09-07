package headroom

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRuntimeURL(t *testing.T) {
	t.Setenv(RuntimeModeEnv, "")
	if got := RuntimeURL(); got != nativeRuntimeURL {
		t.Fatalf("native RuntimeURL() = %q, want %q", got, nativeRuntimeURL)
	}
	t.Setenv(RuntimeModeEnv, RuntimeCompose)
	if got := RuntimeURL(); got != composeRuntimeURL {
		t.Fatalf("compose RuntimeURL() = %q, want %q", got, composeRuntimeURL)
	}
}

func TestRuntimeExecutableUsesPrivateDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	want := filepath.Join(dataDir, "headroom-runtime", "venv", "bin", "headroom")
	if got := RuntimeExecutable(dataDir); got != want {
		t.Fatalf("RuntimeExecutable() = %q, want %q", got, want)
	}
}

func TestRuntimeSupervisorReusesReadyProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != runtimeReadyPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, runtimeReadyPath)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	started := false
	s := RuntimeSupervisor{
		Endpoint: server.URL,
		Client:   server.Client(),
		Start: func(string, ...string) *exec.Cmd {
			started = true
			return nil
		},
		Wait: time.Second,
	}
	cleanup, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if started {
		t.Fatal("Ensure() started a proxy that was already ready")
	}
	cleanup()
}

func TestRuntimeSupervisorStartsExpectedCommandAndCleansUpChild(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "headroom")
	if err := os.WriteFile(executable, []byte("placeholder"), 0o700); err != nil {
		t.Fatalf("write placeholder executable: %v", err)
	}

	var readyChecks atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != runtimeReadyPath {
			t.Fatalf("path = %q, want %q", req.URL.Path, runtimeReadyPath)
		}
		if readyChecks.Add(1) == 1 {
			return nil, errors.New("not ready")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})}

	var gotName string
	var gotArgs []string
	var child *exec.Cmd
	s := RuntimeSupervisor{
		Endpoint:   nativeRuntimeURL,
		Executable: executable,
		Client:     client,
		Start: func(name string, args ...string) *exec.Cmd {
			gotName, gotArgs = name, args
			child = exec.Command("sh", "-c", "while :; do sleep 1; done")
			return child
		},
		Wait: time.Second,
	}
	cleanup, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if gotName != executable {
		t.Fatalf("command executable = %q, want %q", gotName, executable)
	}
	if got := strings.Join(gotArgs, " "); got != "proxy --host 127.0.0.1 --port 8787" {
		t.Fatalf("command arguments = %q", got)
	}
	cleanup()
	if child.ProcessState == nil {
		t.Fatal("cleanup did not terminate the child process")
	}
}

func TestRuntimeSupervisorBootstrapsMissingRuntimeBeforeStarting(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "headroom")
	var readyChecks atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		if readyChecks.Add(1) == 1 {
			return nil, errors.New("not ready")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})}

	bootstrapped := false
	s := RuntimeSupervisor{
		Endpoint:   nativeRuntimeURL,
		Executable: executable,
		Client:     client,
		Bootstrap: func(context.Context) error {
			bootstrapped = true
			return os.WriteFile(executable, []byte("placeholder"), 0o700)
		},
		Start: func(string, ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "while :; do sleep 1; done")
		},
		Wait: time.Second,
	}
	cleanup, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !bootstrapped {
		t.Fatal("Ensure() did not bootstrap the missing runtime")
	}
	cleanup()
}

func TestStartRuntimeSkipsComposeManagedSidecar(t *testing.T) {
	t.Setenv(RuntimeModeEnv, RuntimeCompose)
	cleanup, err := StartRuntime(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	cleanup()
}

func TestRuntimeBootstrapScriptUsesPrivateRuntimeCopy(t *testing.T) {
	dataDir := t.TempDir()
	script := filepath.Join(dataDir, "headroom-runtime", "bootstrap", "ensure-headroom-runtime.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o700); err != nil {
		t.Fatalf("create bootstrap directory: %v", err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
		t.Fatalf("write bootstrap script: %v", err)
	}
	got, err := runtimeBootstrapScript(dataDir)
	if err != nil {
		t.Fatalf("runtimeBootstrapScript() error = %v", err)
	}
	if got != script {
		t.Fatalf("runtimeBootstrapScript() = %q, want %q", got, script)
	}
}
