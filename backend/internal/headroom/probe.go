package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeResult reports whether a Headroom proxy is reachable and behaving like a
// working /v1/compress endpoint. It is returned by Probe and surfaced to the
// dashboard "Test connection" control. Endpoint is always masked so embedded
// credentials and query strings never reach the UI or logs.
type ProbeResult struct {
	OK        bool   `json:"ok"`        // proxy answered like a working compressor
	Reachable bool   `json:"reachable"` // an HTTP response was received at all
	Status    int    `json:"status"`    // HTTP status code (0 when unreachable)
	LatencyMs int64  `json:"latency_ms"`
	Endpoint  string `json:"endpoint"` // masked /v1/compress URL that was probed
	Message   string `json:"message"`  // human-readable result
}

// Probe sends a minimal compression request to the configured proxy and reports
// whether it is up. It never returns an error: every failure is captured in the
// result so the caller can render a status without special-casing.
func (c *Compressor) Probe(ctx context.Context, cfg Config) ProbeResult {
	endpoint := buildCompressEndpoint(cfg.URL)
	res := ProbeResult{Endpoint: maskURL(endpoint)}

	if strings.TrimSpace(cfg.URL) == "" {
		res.Message = "proxy URL is empty"
		return res
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A tiny, valid OpenAI-shaped payload so a working proxy returns messages[].
	payload, _ := json.Marshal(compressRequest{
		Messages: []openAIMessage{{Role: "user", Content: "ping"}},
		Model:    "headroom-probe",
	})

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		res.Message = "could not build request: " + err.Error()
		return res
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := c.client.Do(httpReq)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Message = "not reachable: " + maskErr(err)
		return res
	}
	defer httpResp.Body.Close()

	res.Reachable = true
	res.Status = httpResp.StatusCode

	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		res.Message = fmt.Sprintf("reachable, but returned HTTP %d", httpResp.StatusCode)
		return res
	}

	applyProbeResponse(&res, httpResp.Body)
	return res
}

func applyProbeResponse(res *ProbeResult, body io.Reader) {
	var decoded compressResponse
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		res.Message = "reachable, but response was not valid JSON"
		return
	}
	if decoded.CompressionSkipped {
		res.Message = "reachable, but compression was skipped"
		return
	}
	res.OK = true
	if len(decoded.Messages) == 0 {
		res.Message = "reachable; proxy responded but returned no compressed messages"
		return
	}
	res.Message = "Headroom proxy is running"
}

// maskErr renders an error message with any embedded URL stripped of
// credentials and query string, so probe failures never leak secrets.
func maskErr(err error) string {
	return scrubURLText(err.Error())
}
