package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminTestProxyRejectsURLWithoutHost(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/settings/proxy-test", strings.NewReader(`{"proxyUrl":"http://"}`))
	rec := httptest.NewRecorder()

	s.adminTestProxy(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"ok":false,"error":"invalid proxy URL: host is required"}`, rec.Body.String())
}
