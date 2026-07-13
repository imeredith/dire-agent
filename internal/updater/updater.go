// Package updater securely prepares and applies Dire Agent binary upgrades.
package updater

import (
	"bufio"
	"context"
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

const (
	DefaultLatestBaseURL  = "https://github.com/dire-kiwi/dire-agent/releases/latest/download"
	DefaultReleaseBaseURL = "https://github.com/dire-kiwi/dire-agent/releases/download"
	maxMetadataBytes      = 1 << 20
	maxBinaryBytes        = 512 << 20
)

type Updater struct {
	Client         *http.Client
	LatestBaseURL  string
	ReleaseBaseURL string
	GOOS           string
	GOARCH         string
	Executable     string
}

type Prepared struct {
	Path      string
	Target    string
	Version   string
	AssetName string
	applied   bool
}

func New(executable string) *Updater {
	return &Updater{
		Client:        newHTTPClient(),
		LatestBaseURL: DefaultLatestBaseURL, ReleaseBaseURL: DefaultReleaseBaseURL,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Executable: executable,
	}
}

// Prepare downloads and verifies an upgrade into the executable's directory.
// It does not replace the current binary until Apply is called.
func (u *Updater) Prepare(ctx context.Context, requestedVersion string) (*Prepared, error) {
	if u == nil {
		return nil, errors.New("updater is nil")
	}
	if u.GOOS != "darwin" && u.GOOS != "linux" {
		return nil, fmt.Errorf("unsupported operating system %q", u.GOOS)
	}
	if u.GOARCH != "amd64" && u.GOARCH != "arm64" {
		return nil, fmt.Errorf("unsupported architecture %q", u.GOARCH)
	}
	target, err := canonicalTarget(u.Executable)
	if err != nil {
		return nil, err
	}
	client := u.Client
	if client == nil {
		client = newHTTPClient()
	}
	baseURL := strings.TrimRight(u.LatestBaseURL, "/")
	requestedTag := ""
	if version := strings.TrimSpace(requestedVersion); version != "" && version != "latest" {
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
		if !validVersion(version) {
			return nil, fmt.Errorf("invalid release version %q", version)
		}
		requestedTag = version
		baseURL = strings.TrimRight(u.ReleaseBaseURL, "/") + "/" + version
	}
	if baseURL == "" {
		return nil, errors.New("release URL is empty")
	}
	versionBytes, err := getBytes(ctx, client, baseURL+"/version.txt", maxMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("download release version: %w", err)
	}
	version := strings.TrimSpace(string(versionBytes))
	if !validVersion(version) {
		return nil, errors.New("release version.txt is invalid")
	}
	if requestedTag != "" && version != requestedTag {
		return nil, fmt.Errorf("release metadata says %s, expected %s", version, requestedTag)
	}
	assetBaseURL := baseURL
	if requestedTag == "" {
		releaseBaseURL := strings.TrimRight(u.ReleaseBaseURL, "/")
		if releaseBaseURL == "" {
			return nil, errors.New("release URL is empty")
		}
		assetBaseURL = releaseBaseURL + "/" + version
	}
	assetName := fmt.Sprintf("dire-agent-%s-%s", u.GOOS, u.GOARCH)
	checksums, err := getBytes(ctx, client, assetBaseURL+"/checksums.txt", maxMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("download release checksums: %w", err)
	}
	wantHash, err := checksumFor(checksums, assetName)
	if err != nil {
		return nil, err
	}

	file, err := os.CreateTemp(filepath.Dir(target), ".dire-agent-upgrade-*")
	if err != nil {
		return nil, fmt.Errorf("create upgrade file beside %s: %w", target, err)
	}
	path := file.Name()
	keep := false
	defer func() {
		if !keep {
			file.Close()
			os.Remove(path)
		}
	}()
	if err := downloadAndVerify(ctx, client, assetBaseURL+"/"+assetName, file, wantHash); err != nil {
		return nil, err
	}
	if err := file.Chmod(0o755); err != nil {
		return nil, fmt.Errorf("make downloaded binary executable: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync downloaded binary: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close downloaded binary: %w", err)
	}
	keep = true
	return &Prepared{Path: path, Target: target, Version: version, AssetName: assetName}, nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many release redirects")
			}
			if request.URL.Scheme != "https" {
				return errors.New("refusing non-HTTPS release redirect")
			}
			return nil
		},
	}
}

func validVersion(version string) bool {
	if len(version) < 2 || version[0] != 'v' {
		return false
	}
	for _, character := range version[1:] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._+-", character) {
			continue
		}
		return false
	}
	return true
}

func (p *Prepared) Apply() error {
	if p == nil || p.Path == "" || p.Target == "" {
		return errors.New("prepared upgrade is invalid")
	}
	if p.applied {
		return errors.New("prepared upgrade was already applied")
	}
	if err := os.Rename(p.Path, p.Target); err != nil {
		return fmt.Errorf("replace %s: %w", p.Target, err)
	}
	p.applied = true
	if directory, err := os.Open(filepath.Dir(p.Target)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (p *Prepared) Cleanup() {
	if p != nil && !p.applied && p.Path != "" {
		_ = os.Remove(p.Path)
	}
}

func canonicalTarget(executable string) (string, error) {
	if strings.TrimSpace(executable) == "" {
		return "", errors.New("executable path is empty")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect executable %s: %w", absolute, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("executable is not a regular file: %s", absolute)
	}
	return absolute, nil
}

func getBytes(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "dire-agent-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", url, limit)
	}
	return data, nil
}

func checksumFor(contents []byte, assetName string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || strings.TrimPrefix(fields[len(fields)-1], "*") != assetName {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return zero, fmt.Errorf("invalid SHA-256 checksum for %s", assetName)
		}
		var result [sha256.Size]byte
		copy(result[:], decoded)
		return result, nil
	}
	if err := scanner.Err(); err != nil {
		return zero, fmt.Errorf("read checksums: %w", err)
	}
	return zero, fmt.Errorf("checksums.txt does not contain %s", assetName)
}

func downloadAndVerify(ctx context.Context, client *http.Client, url string, destination *os.File, want [sha256.Size]byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "dire-agent-updater")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download release binary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("download release binary: %s returned %s", url, response.Status)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(response.Body, maxBinaryBytes+1))
	if err != nil {
		return fmt.Errorf("download release binary: %w", err)
	}
	if written > maxBinaryBytes {
		return fmt.Errorf("release binary exceeds the %d-byte limit", maxBinaryBytes)
	}
	got := hash.Sum(nil)
	if !equalBytes(got, want[:]) {
		return fmt.Errorf("SHA-256 mismatch for %s", filepath.Base(url))
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var difference byte
	for i := range a {
		difference |= a[i] ^ b[i]
	}
	return difference == 0
}
