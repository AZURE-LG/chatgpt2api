package service

import (
	"archive/tar"
	"compress/gzip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{name: "older patch", current: "1.2.3", latest: "1.2.4", want: -1},
		{name: "older minor", current: "1.2.9", latest: "1.3.0", want: -1},
		{name: "same with v prefix", current: "v1.2.3", latest: "1.2.3", want: 0},
		{name: "newer", current: "2.0.0", latest: "1.9.9", want: 1},
		{name: "pre release trims suffix", current: "1.2.3-dev", latest: "1.2.3", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compareVersions(tc.current, tc.latest)
			if got != tc.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestValidateUpdateDownloadURL(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/ZyphrZero/chatgpt2api/releases/download/v1.0.0/chatgpt2api.tar.gz",
		"https://objects.githubusercontent.com/github-production-release-asset/example",
	} {
		if err := validateUpdateDownloadURL(raw); err != nil {
			t.Fatalf("validateUpdateDownloadURL(%q) error = %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://github.com/ZyphrZero/chatgpt2api/releases/download/v1.0.0/chatgpt2api.tar.gz",
		"https://example.com/chatgpt2api.tar.gz",
	} {
		if err := validateUpdateDownloadURL(raw); err == nil {
			t.Fatalf("validateUpdateDownloadURL(%q) succeeded, want error", raw)
		}
	}
}

func TestExtractUpdateArchiveFindsRuntimePayload(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "chatgpt2api_1.2.3_linux_amd64.tar.gz")
	if err := writeTestUpdateArchive(archivePath); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	extractDir := filepath.Join(root, "extract")
	if err := extractUpdateArchive(archivePath, extractDir); err != nil {
		t.Fatalf("extractUpdateArchive() error = %v", err)
	}
	wantBinaryName := "chatgpt2api"
	if runtime.GOOS == "windows" {
		wantBinaryName += ".exe"
	}
	if binary, err := findExtractedBinary(extractDir); err != nil || filepath.Base(binary) != wantBinaryName {
		t.Fatalf("findExtractedBinary() = %q, %v", binary, err)
	}
	if webDist, err := findExtractedWebDist(extractDir); err != nil {
		t.Fatalf("findExtractedWebDist() error = %v", err)
	} else if _, err := os.Stat(filepath.Join(webDist, "index.html")); err != nil {
		t.Fatalf("web_dist index missing: %v", err)
	}
}

func TestSafeExtractPathRejectsTraversal(t *testing.T) {
	if _, err := safeExtractPath(t.TempDir(), "../outside"); err == nil {
		t.Fatal("safeExtractPath accepted traversal path")
	}
}

func TestDownloadFileNameIgnoresQuery(t *testing.T) {
	raw := "https://github.com/ZyphrZero/chatgpt2api/releases/download/v1.0.0/chatgpt2api.tar.gz?download=1"
	if got := downloadFileName(raw); got != "chatgpt2api.tar.gz" {
		t.Fatalf("downloadFileName(%q) = %q", raw, got)
	}
	if _, err := url.Parse(raw); err != nil {
		t.Fatalf("test URL invalid: %v", err)
	}
}

func writeTestUpdateArchive(path string) error {
	binaryName := "chatgpt2api"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for name, content := range map[string]string{
		"chatgpt2api_1.2.3_linux_amd64/" + binaryName:        "binary",
		"chatgpt2api_1.2.3_linux_amd64/web_dist/index.html":  "<html></html>",
		"chatgpt2api_1.2.3_linux_amd64/web_dist/assets/a.js": "console.log(1)",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}
	return nil
}
