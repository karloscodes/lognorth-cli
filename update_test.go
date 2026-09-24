package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// releaseServer answers like GitHub releases: latest redirects to the tag,
// and latest/download serves the binary and its checksums.
func releaseServer(t *testing.T, tag string, binary, checksum []byte) {
	t.Helper()
	asset := fmt.Sprintf("north-%s-%s", runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			http.Redirect(w, r, "/tag/"+tag, http.StatusFound)
		case "/latest/download/" + asset:
			w.Write(binary)
		case "/latest/download/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", checksum, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := releases
	releases = srv.URL
	t.Cleanup(func() { releases = old })
}

func sha(b []byte) []byte {
	sum := sha256.Sum256(b)
	return []byte(hex.EncodeToString(sum[:]))
}

func TestUpdate(t *testing.T) {
	asset := fmt.Sprintf("north-%s-%s", runtime.GOOS, runtime.GOARCH)
	installed := func(t *testing.T) string {
		t.Helper()
		exe := filepath.Join(t.TempDir(), "north")
		os.WriteFile(exe, []byte("old north"), 0o755)
		return exe
	}

	t.Run("finds the latest version from the release redirect", func(t *testing.T) {
		releaseServer(t, "v9.9.9", nil, nil)

		latest, err := latestVersion()

		if err != nil || latest != "9.9.9" {
			t.Errorf("latest = %q, %v; want 9.9.9", latest, err)
		}
	})

	t.Run("replaces the binary with a download that matches its checksum", func(t *testing.T) {
		newNorth := []byte("new north")
		releaseServer(t, "v9.9.9", newNorth, sha(newNorth))
		exe := installed(t)

		err := replaceBinary(exe, asset)

		got, _ := os.ReadFile(exe)
		info, _ := os.Stat(exe)
		if err != nil || string(got) != "new north" || info.Mode().Perm() != 0o755 {
			t.Errorf("err = %v, binary = %q, mode = %v; want the new executable", err, got, info.Mode())
		}
	})

	t.Run("keeps the old binary when the checksum does not match", func(t *testing.T) {
		releaseServer(t, "v9.9.9", []byte("tampered"), sha([]byte("new north")))
		exe := installed(t)

		err := replaceBinary(exe, asset)

		got, _ := os.ReadFile(exe)
		entries, _ := os.ReadDir(filepath.Dir(exe))
		if err == nil || string(got) != "old north" || len(entries) != 1 {
			t.Errorf("err = %v, binary = %q, files = %d; want an error, the old binary, and no leftovers", err, got, len(entries))
		}
	})

	t.Run("says so when the release has no build for this machine", func(t *testing.T) {
		releaseServer(t, "v9.9.9", nil, nil)

		err := replaceBinary(installed(t), "north-plan9-mips")

		if err == nil {
			t.Error("want an error for a missing build")
		}
	})
}
