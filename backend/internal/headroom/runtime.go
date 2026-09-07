package headroom

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// RuntimeModeEnv selects the deployment-owned proxy endpoint. It is set only
	// by the Compose manifests; native installs manage a loopback proxy.
	RuntimeModeEnv = "KEIROUTER_HEADROOM_RUNTIME"
	RuntimeCompose = "compose"

	nativeRuntimeURL  = "http://127.0.0.1:8787"
	composeRuntimeURL = "http://headroom:8787"
	runtimeReadyPath  = "/readyz"
	runtimeStartWait  = 10 * time.Second
)

// RuntimeURL returns the private Headroom proxy address for the active
// deployment. It deliberately is not a dashboard setting: both addresses are
// owned by KeiRouter's supported runtimes.
func RuntimeURL() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(RuntimeModeEnv)), RuntimeCompose) {
		return composeRuntimeURL
	}
	return nativeRuntimeURL
}

// RuntimeExecutable returns the provisioned native Headroom executable. The
// installer creates this isolated virtual environment under the data directory.
func RuntimeExecutable(dataDir string) string {
	return filepath.Join(dataDir, "headroom-runtime", "venv", "bin", "headroom")
}

// RuntimeSupervisor starts a native proxy only when one is not already ready.
// Its cleanup function terminates only the child process that it created.
type RuntimeSupervisor struct {
	Endpoint   string
	Executable string
	Client     *http.Client
	Start      func(string, ...string) *exec.Cmd
	Bootstrap  func(context.Context) error
	Wait       time.Duration
}

// StartRuntime prepares the native Headroom process for a serving lifecycle.
// Compose owns its sidecar independently, while native startup stays fail-open:
// a missing or unhealthy proxy never prevents KeiRouter itself from starting.
func StartRuntime(ctx context.Context, dataDir string, log *slog.Logger) (func(), error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(RuntimeModeEnv)), RuntimeCompose) {
		return func() {}, nil
	}
	if log == nil {
		log = slog.Default()
	}
	if runtime.GOOS == "windows" {
		err := errors.New("native Headroom provisioning is supported on macOS and Linux; use Docker on Windows")
		log.Warn("headroom runtime unavailable; compression will fail open", "error", err)
		return func() {}, err
	}
	s := RuntimeSupervisor{
		Endpoint:   RuntimeURL(),
		Executable: RuntimeExecutable(dataDir),
		Client:     &http.Client{Timeout: time.Second},
		Start:      exec.Command,
		Bootstrap: func(ctx context.Context) error {
			return bootstrapRuntime(ctx, dataDir)
		},
		Wait: runtimeStartWait,
	}
	cleanup, err := s.Ensure(ctx)
	if err != nil {
		log.Warn("headroom runtime unavailable; compression will fail open", "error", err)
		return func() {}, err
	}
	return cleanup, nil
}

// Ensure reuses a ready proxy or starts the provisioned executable and waits
// for readiness. It is intentionally small and injectable for lifecycle tests.
func (s RuntimeSupervisor) Ensure(ctx context.Context) (func(), error) {
	if s.ready(ctx) {
		return func() {}, nil
	}
	s.applyDefaults()
	if err := s.bootstrapIfNeeded(ctx); err != nil {
		return nil, err
	}
	if err := s.validateExecutable(); err != nil {
		return nil, err
	}
	cmd, err := s.startProxy()
	if err != nil {
		return nil, err
	}
	cleanup := cleanupCommand(cmd)
	if err := s.waitUntilReady(ctx); err != nil {
		cleanup()
		return nil, err
	}
	return cleanup, nil
}

func (s RuntimeSupervisor) bootstrapIfNeeded(ctx context.Context) error {
	if _, err := os.Stat(s.Executable); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect headroom runtime at %q: %w", s.Executable, err)
	}
	if s.Bootstrap == nil {
		return nil
	}
	if err := s.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap headroom runtime: %w", err)
	}
	return nil
}

func bootstrapRuntime(ctx context.Context, dataDir string) error {
	script, err := runtimeBootstrapScript(dataDir)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, script)
	cmd.Env = append(os.Environ(), "KEIROUTER_DATA__DIR="+dataDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run runtime bootstrap: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runtimeBootstrapScript(dataDir string) (string, error) {
	candidates := []string{
		filepath.Join(dataDir, "headroom-runtime", "bootstrap", "ensure-headroom-runtime.sh"),
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "scripts", "ensure-headroom-runtime.sh"))
	}
	if workingDir, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(workingDir, "scripts", "ensure-headroom-runtime.sh"),
			filepath.Join(workingDir, "..", "scripts", "ensure-headroom-runtime.sh"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("runtime bootstrap script unavailable; rerun the native installer or start from a KeiRouter source checkout")
}

func (s *RuntimeSupervisor) applyDefaults() {
	if s.Start == nil {
		s.Start = exec.Command
	}
	if s.Client == nil {
		s.Client = &http.Client{Timeout: time.Second}
	}
	if s.Wait <= 0 {
		s.Wait = runtimeStartWait
	}
}

func (s RuntimeSupervisor) validateExecutable() error {
	if strings.TrimSpace(s.Executable) == "" {
		return errors.New("headroom runtime executable is not configured")
	}
	if _, err := os.Stat(s.Executable); err != nil {
		return fmt.Errorf("headroom runtime is not provisioned at %q: %w", s.Executable, err)
	}
	return nil
}

func (s RuntimeSupervisor) startProxy() (*exec.Cmd, error) {
	cmd := s.Start(s.Executable, "proxy", "--host", "127.0.0.1", "--port", "8787")
	if cmd == nil {
		return nil, errors.New("headroom runtime command was not created")
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start headroom runtime: %w", err)
	}
	return cmd, nil
}

func cleanupCommand(cmd *exec.Cmd) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if cmd.Process == nil {
				return
			}
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
		})
	}
}

func (s RuntimeSupervisor) waitUntilReady(ctx context.Context) error {
	deadline := time.NewTimer(s.Wait)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if s.ready(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("headroom runtime did not become ready")
		case <-tick.C:
		}
	}
}

func (s RuntimeSupervisor) ready(ctx context.Context) bool {
	if strings.TrimSpace(s.Endpoint) == "" || s.Client == nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.Endpoint, "/")+runtimeReadyPath, nil)
	if err != nil {
		return false
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}
