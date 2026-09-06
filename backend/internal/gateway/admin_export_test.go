package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeDatabaseExportPassphraseReadsOnlyJSONBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/settings/database/export?passphrase=url-secret", strings.NewReader(`{"passphrase":" body-secret "}`))

	passphrase, ok := decodeDatabaseExportPassphrase(rec, req)

	require.True(t, ok)
	require.Equal(t, "body-secret", passphrase)
	require.Empty(t, rec.Body.String())
}

func TestDecodeDatabaseExportPassphraseRejectsUnknownFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/settings/database/export", strings.NewReader(`{"passphrase":"secret","unexpected":true}`))

	_, ok := decodeDatabaseExportPassphrase(rec, req)

	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
