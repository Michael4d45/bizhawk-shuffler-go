package installer

import (
	"runtime"
	"strings"
	"testing"
)

func TestFindBizHawkReleaseAssetLinuxTarball(t *testing.T) {
	rel := &Release{
		TagName: "2.11.1",
		Assets: []Asset{
			{Name: "BizHawk-2.11.1-win-x64.zip", DownloadURL: "https://example.com/win.zip"},
			{Name: "BizHawk-2.11.1-linux-x64.tar.gz", DownloadURL: "https://example.com/linux.tar.gz"},
		},
	}
	if runtime.GOOS != "linux" {
		t.Skip("linux asset preference is only on GOOS=linux")
	}
	asset := FindBizHawkReleaseAsset(rel)
	if asset == nil {
		t.Fatal("expected asset")
	}
	if !strings.HasSuffix(asset.Name, ".tar.gz") {
		t.Fatalf("expected linux tar.gz, got %q", asset.Name)
	}
}

func TestFallbackBizHawkDownloadURLLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux fallback URL only on GOOS=linux")
	}
	url := fallbackBizHawkDownloadURL("2.11.1")
	if !strings.HasSuffix(url, "BizHawk-2.11.1-linux-x64.tar.gz") {
		t.Fatalf("unexpected fallback URL: %s", url)
	}
}

func TestFallbackBizHawkDownloadURLWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows fallback URL only on GOOS=windows")
	}
	url := fallbackBizHawkDownloadURL("2.11.1")
	if !strings.HasSuffix(url, "BizHawk-2.11.1-win-x64.zip") {
		t.Fatalf("unexpected fallback URL: %s", url)
	}
}
