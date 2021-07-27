package gcsemu

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fullstorydev/emulators/storage/gcsclient"
	"gotest.tools/v3/assert"
)

func TestFileStore(t *testing.T) {
	// Setup an on-disk emulator.
	gcsDir := filepath.Join(os.TempDir(), fmt.Sprintf("gcsemu-test-%d", time.Now().Unix()))
	gcsEmu := NewGcsEmu(Options{
		Store:   NewFileStore(gcsDir),
		Verbose: true,
		Log: func(err error, fmt string, args ...interface{}) {
			if err != nil {
				fmt = "ERROR: " + fmt + ": %s"
				args = append(args, err)
			}
			t.Logf(fmt, args...)
		},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/", gcsEmu.Handler)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("about to method=%s host=%s u=%s", r.Method, r.Host, r.URL)
		gcsEmu.Handler(w, r)
	}))
	t.Cleanup(svr.Close)

	host := strings.TrimPrefix(svr.URL, "http://")
	gcsClient, err := gcsclient.NewTestClientWithHost(context.Background(), host)
	assert.NilError(t, err)

	bh := BucketHandle{
		Name:         "file-bucket",
		BucketHandle: gcsClient.Bucket("file-bucket"),
	}

	t.Parallel()
	initBucket(t, bh)
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.f(t, bh)
		})
	}

	t.Run("RawHttp", func(t *testing.T) {
		t.Parallel()
		testRawHttp(t, bh, host)
	})
}
