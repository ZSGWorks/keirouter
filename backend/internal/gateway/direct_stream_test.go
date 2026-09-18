package gateway

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestCopySanitizedStream_PassthroughSSE(t *testing.T) {
	src := strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: [DONE]\n\n")
	var dst bytes.Buffer
	n, err := copySanitizedStream(&dst, src, core.DialectOpenAI, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(dst.Len()) || dst.Len() == 0 {
		t.Fatalf("written %d, buffer %d", n, dst.Len())
	}
	if !strings.Contains(dst.String(), "hi") {
		t.Fatalf("payload lost: %q", dst.String())
	}
}

func TestCopySanitizedNDJSON_OversizedLineRemainderPassed(t *testing.T) {
	// One NDJSON line far past ndjsonOversizeCap.
	content := strings.Repeat("z", ndjsonOversizeCap+512*1024)
	src := strings.NewReader(`{"message":{"content":"` + content + `"}}` + "\n")
	var dst bytes.Buffer
	_, err := copySanitizedStream(&dst, src, core.DialectOllama, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dst.String(), content) {
		t.Fatalf("oversized remainder lost (%d bytes out)", dst.Len())
	}
}

func TestGatewayStreamReaderPoolReuse(t *testing.T) {
	r1 := gatewayStreamReaderPool.Get().(*bufio.Reader)
	gatewayStreamReaderPool.Put(r1)
	r2 := gatewayStreamReaderPool.Get().(*bufio.Reader)
	if r2 != r1 {
		t.Log("pool returned a different reader (GC window); acceptable")
	}
	gatewayStreamReaderPool.Put(r2)
}

func TestCopySanitizedStream_ProviderErrorReplaced(t *testing.T) {
	src := strings.NewReader("data: {\"error\":{\"message\":\"upstream exploded\"}}\n\n")
	var dst bytes.Buffer
	if _, err := copySanitizedStream(&dst, src, core.DialectOpenAI, nil); err == nil {
		t.Fatal("expected provider error propagation")
	}
	if strings.Contains(dst.String(), "upstream exploded") {
		t.Fatal("provider error leaked to client")
	}
}
