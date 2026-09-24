package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// releases is where north downloads itself from. Tests point it elsewhere.
var releases = "https://github.com/karloscodes/lognorth-cli/releases"

// update is north update: it replaces this binary with the latest release,
// checked against the release checksums, the same way install.sh does.
func update(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: north update")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}

	latest, err := latestVersion()
	if err != nil {
		return err
	}
	if latest == version {
		fmt.Printf("north %s is the latest.\n", version)
		return nil
	}

	fmt.Printf("Updating north %s to %s...\n", version, latest)
	if err := replaceBinary(exe, fmt.Sprintf("north-%s-%s", runtime.GOOS, runtime.GOARCH)); err != nil {
		return err
	}
	fmt.Printf("%s north %s\n", colorsFor(os.Stdout).lime("✓"), latest)
	return nil
}

// latestVersion reads the tag GitHub redirects releases/latest to.
func latestVersion() (string, error) {
	c := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := c.Get(releases + "/latest")
	if err != nil {
		return "", fmt.Errorf("could not reach GitHub: %w", err)
	}
	res.Body.Close()
	tag := filepath.Base(res.Header.Get("Location")) // .../releases/tag/v0.2.1
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("could not find the latest release (GitHub answered %s)", res.Status)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// replaceBinary downloads asset next to exe, checks its checksum, and renames
// it over exe. The rename is atomic, so a failed update leaves the old north.
func replaceBinary(exe, asset string) error {
	want, err := checksumOf(asset)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".north-update-")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w. Run the installer again instead", exe, err)
	}
	defer os.Remove(tmp.Name()) // a no-op after the rename

	res, err := httpGet(releases + "/latest/download/" + asset)
	if err != nil {
		tmp.Close()
		return err
	}
	defer res.Body.Close()
	sum := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, sum), res.Body)
	tmp.Close()
	if err != nil {
		return fmt.Errorf("the download stopped: %w", err)
	}
	if hex.EncodeToString(sum.Sum(nil)) != want {
		return errors.New("the download does not match its checksum. Try again")
	}

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}

// checksumOf finds asset's sha256 in the release's checksums.txt.
func checksumOf(asset string) (string, error) {
	res, err := httpGet(releases + "/latest/download/checksums.txt")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	lines := bufio.NewScanner(res.Body)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) == 2 && fields[1] == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("the latest release has no build for %s", asset)
}

func httpGet(url string) (*http.Response, error) {
	c := &http.Client{Timeout: 5 * time.Minute}
	res, err := c.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", url, err)
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("could not download %s: %s", url, res.Status)
	}
	return res, nil
}
