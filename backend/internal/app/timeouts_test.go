package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeTimeoutSettings struct {
	value string
	err   error
}

func (f fakeTimeoutSettings) Get(_ context.Context, key string) (string, error) {
	if key != endpointSettingsStoreKey {
		return "", errors.New("unexpected settings key")
	}
	return f.value, f.err
}

func TestInitialTimeoutValuesUsesPersistedSettings(t *testing.T) {
	stall, header, request := initialTimeoutValues(
		context.Background(),
		fakeTimeoutSettings{value: `{"stream_stall_timeout_ms":300000,"response_header_timeout_ms":90000,"request_timeout_ms":600000}`},
		time.Minute,
		30*time.Second,
		2*time.Minute,
	)

	if stall != 5*time.Minute || header != 90*time.Second || request != 10*time.Minute {
		t.Fatalf("initialTimeoutValues() = (%v, %v, %v), want (5m, 90s, 10m)", stall, header, request)
	}
}

func TestInitialTimeoutValuesKeepsFallbacksForMissingOrMalformedSettings(t *testing.T) {
	wantStall := 5 * time.Minute
	wantHeader := time.Minute
	wantRequest := 5 * time.Minute

	for _, tt := range []struct {
		name     string
		settings timeoutSettingsReader
	}{
		{name: "missing", settings: fakeTimeoutSettings{err: errors.New("not found")}},
		{name: "malformed", settings: fakeTimeoutSettings{value: `{not-json`}},
		{name: "nil repo", settings: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stall, header, request := initialTimeoutValues(
				context.Background(), tt.settings, wantStall, wantHeader, wantRequest,
			)
			if stall != wantStall || header != wantHeader || request != wantRequest {
				t.Fatalf("initialTimeoutValues() = (%v, %v, %v), want fallbacks", stall, header, request)
			}
		})
	}
}

func TestInitialTimeoutValuesSupportsPartialSettings(t *testing.T) {
	stall, header, request := initialTimeoutValues(
		context.Background(),
		fakeTimeoutSettings{value: `{"stream_stall_timeout_ms":180000}`},
		5*time.Minute,
		time.Minute,
		5*time.Minute,
	)

	if stall != 3*time.Minute || header != time.Minute || request != 5*time.Minute {
		t.Fatalf("initialTimeoutValues() = (%v, %v, %v), want (3m, 1m, 5m)", stall, header, request)
	}
}
