package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxmind/geoipupdate/v6/pkg/geoipupdate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultipleDatabaseDownload(t *testing.T) {
	databaseContent := "database content goes here"

	server := httptest.NewServer(
		http.HandlerFunc(
			func(rw http.ResponseWriter, r *http.Request) {
				err := r.ParseForm()
				require.NoError(t, err, "parse form")

				if strings.HasPrefix(r.URL.Path, "/geoip/databases") {
					buf := &bytes.Buffer{}
					gzWriter := gzip.NewWriter(buf)
					md5Writer := md5.New()
					multiWriter := io.MultiWriter(gzWriter, md5Writer)
					_, err := multiWriter.Write([]byte(
						databaseContent + " " + r.URL.Path,
					))
					require.NoError(t, err)
					err = gzWriter.Close()
					require.NoError(t, err)

					rw.Header().Set(
						"X-Database-MD5",
						hex.EncodeToString(md5Writer.Sum(nil)),
					)
					rw.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))

					_, err = rw.Write(buf.Bytes())
					require.NoError(t, err)

					return
				}

				rw.WriteHeader(http.StatusBadRequest)
			},
		),
	)
	defer server.Close()

	tempDir := t.TempDir()

	config := &geoipupdate.Config{
		AccountID:         123,
		DatabaseDirectory: tempDir,
		EditionIDs:        []string{"GeoLite2-City", "GeoLite2-Country"},
		LicenseKey:        "testing",
		LockFile:          filepath.Join(tempDir, ".geoipupdate.lock"),
		URL:               server.URL,
		Parallelism:       1,
	}

	logOutput := &bytes.Buffer{}
	log.SetOutput(logOutput)

	client := geoipupdate.NewClient(config)
	err := client.Run(context.Background())
	require.NoError(t, err, "run successfully")

	assert.Equal(t, "", logOutput.String(), "no logged output")

	for _, editionID := range config.EditionIDs {
		path := filepath.Join(config.DatabaseDirectory, editionID+".mmdb")
		buf, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err, "read file")
		assert.Equal(
			t,
			databaseContent+" /geoip/databases/"+editionID+"/update",
			string(buf),
			"correct database",
		)
	}
}
