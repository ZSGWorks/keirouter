package gateway

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func postMultipartFile(t *testing.T, handler http.HandlerFunc, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestAnalyze9routerSQLiteRejectsNonSQLiteUpload(t *testing.T) {
	s := &Server{}
	rec := postMultipartFile(t, s.adminAnalyze9routerSQLite, "not.sqlite", []byte("this is definitely not a sqlite database"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAnalyze9routerSQLiteCountsRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nine.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE providerNodes (id TEXT); INSERT INTO providerNodes (id) VALUES ('a'), ('b');`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s := &Server{}
	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	rec := postMultipartFile(t, s.adminAnalyze9routerSQLite, "nine.sqlite", content)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var counts map[string]int64
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &counts))
	require.Equal(t, int64(2), counts["providerNodes"])
}

func TestImport9routerConnectionsUsesDeterministicIDs(t *testing.T) {
	s, db := newBulkTestServer(t)
	payload := []byte(`[{"id":"source-1","provider":"kiro","authType":"api_key","name":"Kiro key","accessToken":"headless-token","providerSpecificData":{"authMethod":"api_key","profileArn":"arn:aws:codewhisperer:us-east-1:123:profile/ABC","region":"us-east-1"}}]`)
	doc := map[string]json.RawMessage{"providerConnections": payload}

	first := &foreignImportResult{}
	s.importN9routerConnections(context.Background(), doc, first, nil)
	require.Equal(t, 1, first.Accounts, first.Errors)

	accounts, err := db.Accounts().ListByProvider(context.Background(), adminTenant, "kiro")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, n9IDPrefix+"source-1", accounts[0].ID)

	second := &foreignImportResult{}
	s.importN9routerConnections(context.Background(), doc, second, nil)

	accounts, err = db.Accounts().ListByProvider(context.Background(), adminTenant, "kiro")
	require.NoError(t, err)
	require.Len(t, accounts, 1, "re-import must not duplicate the deterministic-id account")
	require.Zero(t, second.Accounts)
}

func TestImport9routerSettingsIgnoresHeadroomURL(t *testing.T) {
	s, db := newBulkTestServer(t)
	s.settings = db.Settings()

	doc := map[string]json.RawMessage{
		"settings": []byte(`[{"data":{"headroomEnabled":true,"headroomUrl":"https://legacy.example","headroomCodeAware":true}}]`),
	}
	res := &foreignImportResult{}
	s.import9routerSettings(context.Background(), doc, res, false)
	require.Empty(t, res.Errors)

	es := s.loadEndpointSettings(context.Background())
	require.True(t, es.HeadroomEnabled)
	require.True(t, es.HeadroomCompressUserMessages)

	raw, err := s.settings.Get(context.Background(), endpointSettingsKey)
	require.NoError(t, err)
	require.NotContains(t, raw, "headroom_url", "upstream headroomUrl must not be persisted in this fork")
}
