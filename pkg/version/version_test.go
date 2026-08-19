package version

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/eryajf/kite-desktop/pkg/common"
	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "with v prefix", input: "v1.2.3", want: "1.2.3"},
		{name: "without v prefix", input: "1.2.3", want: "1.2.3"},
		{name: "invalid", input: "not-a-version", wantErr: true},
		{name: "empty", input: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSemver(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("unexpected version: want %q, got %q", tt.want, got.String())
			}
		})
	}
}

func TestGetVersionWithoutVersionCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origVersion := Version
	origBuildDate := BuildDate
	origCommitID := CommitID
	origEnableVersionCheck := common.EnableVersionCheck
	origUpdateSource := common.UpdateSource
	t.Cleanup(func() {
		Version = origVersion
		BuildDate = origBuildDate
		CommitID = origCommitID
		common.EnableVersionCheck = origEnableVersionCheck
		common.UpdateSource = origUpdateSource
	})

	Version = "1.2.3"
	BuildDate = "2026-03-27"
	CommitID = "abc123"
	common.EnableVersionCheck = false
	common.UpdateSource = UpdateSourceAuto

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/version", nil)

	GetVersion(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}

	var got VersionInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Version != "1.2.3" || got.BuildDate != "2026-03-27" || got.CommitID != "abc123" {
		t.Fatalf("unexpected version info: %#v", got)
	}
	if got.HasNew || got.Release != "" {
		t.Fatalf("unexpected update fields: %#v", got)
	}
}

func TestGetVersionWithCachedUpdateResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origVersion := Version
	origBuildDate := BuildDate
	origCommitID := CommitID
	origEnableVersionCheck := common.EnableVersionCheck
	origUpdateSource := common.UpdateSource
	origCachedUpdateResult := cachedUpdateResult
	origCachedUpdateKey := cachedUpdateKey
	origLastUpdateFetch := lastUpdateFetch
	t.Cleanup(func() {
		Version = origVersion
		BuildDate = origBuildDate
		CommitID = origCommitID
		common.EnableVersionCheck = origEnableVersionCheck
		common.UpdateSource = origUpdateSource
		cachedUpdateResult = origCachedUpdateResult
		cachedUpdateKey = origCachedUpdateKey
		lastUpdateFetch = origLastUpdateFetch
	})

	Version = "1.2.3"
	BuildDate = "2026-03-27"
	CommitID = "abc123"
	common.EnableVersionCheck = true
	common.UpdateSource = UpdateSourceAuto
	cachedUpdateResult = updateCheckResult{
		comparison:    UpdateComparisonUpdateAvailable,
		latestVersion: "1.2.4",
		releaseURL:    "https://example.com/releases/v1.2.4",
		checkedAt:     time.Now(),
	}
	cachedUpdateKey = buildUpdateCacheKey(Version, UpdateSourceAuto)
	lastUpdateFetch = time.Now()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/version", nil)

	GetVersion(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}

	var got VersionInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if !got.HasNew || got.Release != "https://example.com/releases/v1.2.4" {
		t.Fatalf("unexpected update fields: %#v", got)
	}
}

func TestCheckForUpdateShortCircuitsWithoutNetwork(t *testing.T) {
	origCachedUpdateResult := cachedUpdateResult
	origCachedUpdateKey := cachedUpdateKey
	origLastUpdateFetch := lastUpdateFetch
	t.Cleanup(func() {
		cachedUpdateResult = origCachedUpdateResult
		cachedUpdateKey = origCachedUpdateKey
		lastUpdateFetch = origLastUpdateFetch
	})

	cachedUpdateResult = updateCheckResult{
		comparison:    UpdateComparisonUpdateAvailable,
		latestVersion: "1.2.4",
		releaseURL:    "https://example.com/releases/v1.2.4",
		checkedAt:     time.Now(),
	}
	cachedUpdateKey = buildUpdateCacheKey("1.2.3", UpdateSourceAuto)
	lastUpdateFetch = time.Now()

	got, err := checkForUpdate(context.Background(), "1.2.3", false, UpdateSourceAuto)
	if err != nil {
		t.Fatalf("checkForUpdate() error = %v", err)
	}
	if got.comparison != UpdateComparisonUpdateAvailable || got.releaseURL != "https://example.com/releases/v1.2.4" || got.latestVersion != "1.2.4" {
		t.Fatalf("unexpected cached result: %#v", got)
	}
}

func TestCheckForUpdateSkipsBlankAndDevVersions(t *testing.T) {
	got, err := checkForUpdate(context.Background(), "   ", false, UpdateSourceAuto)
	if err != nil {
		t.Fatalf("blank version returned error: %v", err)
	}
	if got.comparison != UpdateComparisonUncomparable {
		t.Fatalf("blank version comparison = %q, want %q", got.comparison, UpdateComparisonUncomparable)
	}

	got, err = checkForUpdate(context.Background(), "dev", false, UpdateSourceAuto)
	if err != nil {
		t.Fatalf("dev version returned error: %v", err)
	}
	if got.comparison != UpdateComparisonUncomparable {
		t.Fatalf("dev version comparison = %q, want %q", got.comparison, UpdateComparisonUncomparable)
	}
}

func TestCheckForUpdateComparisonStates(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		remoteTag      string
		wantComparison UpdateComparison
	}{
		{name: "update available", currentVersion: "1.2.3", remoteTag: "v1.2.4", wantComparison: UpdateComparisonUpdateAvailable},
		{name: "up to date", currentVersion: "1.2.4", remoteTag: "v1.2.4", wantComparison: UpdateComparisonUpToDate},
		{name: "local newer", currentVersion: "1.2.5", remoteTag: "v1.2.4", wantComparison: UpdateComparisonLocalNewer},
		{name: "current uncomparable", currentVersion: "build-main", remoteTag: "v1.2.4", wantComparison: UpdateComparisonUncomparable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetUpdateCheckerState(t)
			versionCheckClient = stubVersionCheckClient(releasePayload(tt.remoteTag))
			got, err := checkForUpdate(context.Background(), tt.currentVersion, true, UpdateSourceAuto)
			if err != nil {
				t.Fatalf("checkForUpdate() error = %v", err)
			}
			if got.comparison != tt.wantComparison {
				t.Fatalf("comparison = %q, want %q", got.comparison, tt.wantComparison)
			}
		})
	}
}

func TestCheckForUpdateFetchesLatestRelease(t *testing.T) {
	resetUpdateCheckerState(t)

	versionCheckClient = stubVersionCheckClient(releasePayload("v1.2.4"))

	got, err := checkForUpdate(context.Background(), "1.2.3", true, UpdateSourceAuto)
	if err != nil {
		t.Fatalf("checkForUpdate() error = %v", err)
	}
	if got.comparison != UpdateComparisonUpdateAvailable {
		t.Fatalf("comparison = %q, want %q", got.comparison, UpdateComparisonUpdateAvailable)
	}
	if got.latestVersion != "1.2.4" {
		t.Fatalf("latestVersion = %q, want %q", got.latestVersion, "1.2.4")
	}
	if got.releaseURL != "https://github.com/chengsn/kite-desktop/releases/tag/v1.2.4" {
		t.Fatalf("releaseURL = %q, want release url", got.releaseURL)
	}
	if got.releaseNotes != "Release notes" {
		t.Fatalf("releaseNotes = %q, want %q", got.releaseNotes, "Release notes")
	}
	if got.publishedAt != "2026-04-07T14:12:00Z" {
		t.Fatalf("publishedAt = %q, want %q", got.publishedAt, "2026-04-07T14:12:00Z")
	}
	if got.checkedAt.IsZero() {
		t.Fatalf("checkedAt should not be zero")
	}
}

func TestMatchReleaseAsset(t *testing.T) {
	assets := []githubReleaseAsset{
		{Name: "Kite-v1.2.4-macos-intel.dmg", BrowserDownloadURL: "https://example.com/intel-dmg"},
		{Name: "Kite-v1.2.4-macos-arm64.zip", BrowserDownloadURL: "https://example.com/zip", Size: 1234},
		{Name: "Kite-v1.2.4-macos-apple-silicon.dmg", BrowserDownloadURL: "https://example.com/arm-dmg"},
		{Name: "Kite-v1.2.4-windows-amd64-installer.exe", BrowserDownloadURL: "https://example.com/exe", Size: 4321},
	}

	macAsset := matchReleaseAsset(assets, "darwin", "arm64")
	if macAsset == nil || macAsset.Name != "Kite-v1.2.4-macos-arm64.zip" {
		t.Fatalf("unexpected mac asset: %#v", macAsset)
	}

	windowsAsset := matchReleaseAsset(assets, "windows", "amd64")
	if windowsAsset == nil || windowsAsset.Name != "Kite-v1.2.4-windows-amd64-installer.exe" {
		t.Fatalf("unexpected windows asset: %#v", windowsAsset)
	}

	intelAsset := matchReleaseAsset(assets, "darwin", "amd64")
	if intelAsset == nil || intelAsset.Name != "Kite-v1.2.4-macos-intel.dmg" {
		t.Fatalf("unexpected intel mac asset: %#v", intelAsset)
	}

	linuxAsset := matchReleaseAsset(assets, "linux", "amd64")
	if linuxAsset != nil {
		t.Fatalf("expected linux asset to be nil, got %#v", linuxAsset)
	}
}

func TestCheckUpdateReturnsLatestRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origVersion := Version
	origUpdateSource := common.UpdateSource
	t.Cleanup(func() {
		Version = origVersion
		common.UpdateSource = origUpdateSource
	})

	resetUpdateCheckerState(t)
	Version = "v1.2.3"
	common.UpdateSource = UpdateSourceAuto
	versionCheckClient = stubVersionCheckClient(releasePayload("v1.2.4"))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/version/check-update", bytes.NewBufferString(`{"force":true}`))
	c.Request.Header.Set("Content-Type", "application/json")

	CheckUpdate(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var got UpdateCheckInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.CurrentVersion != "1.2.3" {
		t.Fatalf("CurrentVersion = %q, want %q", got.CurrentVersion, "1.2.3")
	}
	if got.LatestVersion != "1.2.4" {
		t.Fatalf("LatestVersion = %q, want %q", got.LatestVersion, "1.2.4")
	}
	if got.Comparison != UpdateComparisonUpdateAvailable {
		t.Fatalf("Comparison = %q, want %q", got.Comparison, UpdateComparisonUpdateAvailable)
	}
	if !got.HasNew || got.Release != "https://github.com/chengsn/kite-desktop/releases/tag/v1.2.4" {
		t.Fatalf("unexpected update payload: %#v", got)
	}
	if got.ReleaseNotes != "Release notes" || got.PublishedAt != "2026-04-07T14:12:00Z" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
	if got.CheckedAt == "" {
		t.Fatalf("CheckedAt should not be empty")
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && (!got.AssetAvailable || got.Asset == nil) {
		t.Fatalf("expected darwin arm64 asset to be available: %#v", got)
	}
}

func TestUpdateSourceRewrite(t *testing.T) {
	releaseURL := "https://github.com/chengsn/kite-desktop/releases/tag/v0.1.10"
	downloadURL := "https://github.com/chengsn/kite-desktop/releases/download/v0.1.10/Kite-v0.1.10-macos-apple-silicon.dmg"

	if got := RewriteReleaseURLBySource(releaseURL, UpdateSourceCNB); got != "https://cnb.cool/eryajf/kite-desktop/-/releases/tag/v0.1.10" {
		t.Fatalf("RewriteReleaseURLBySource(cnb) = %q", got)
	}
	if got := RewriteAssetDownloadURLBySource(downloadURL, UpdateSourceCNB); got != "https://cnb.cool/eryajf/kite-desktop/-/releases/download/v0.1.10/Kite-v0.1.10-macos-apple-silicon.dmg" {
		t.Fatalf("RewriteAssetDownloadURLBySource(cnb) = %q", got)
	}
}

func TestAlternateMirrorDownloadURL(t *testing.T) {
	githubURL := "https://github.com/chengsn/kite-desktop/releases/download/v0.1.10/Kite-v0.1.10-macos-apple-silicon.dmg"
	cnbURL := "https://cnb.cool/eryajf/kite-desktop/-/releases/download/v0.1.10/Kite-v0.1.10-macos-apple-silicon.dmg"

	if got := AlternateMirrorDownloadURL(githubURL); got != cnbURL {
		t.Fatalf("AlternateMirrorDownloadURL(github) = %q, want %q", got, cnbURL)
	}
	if got := AlternateMirrorDownloadURL(cnbURL); got != githubURL {
		t.Fatalf("AlternateMirrorDownloadURL(cnb) = %q, want %q", got, githubURL)
	}
}

func stubVersionCheckClient(body string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(bytes.NewBufferString(body)),
			}, nil
		}),
	}
}

func releasePayload(tag string) string {
	return `{
		"tag_name":"` + tag + `",
		"html_url":"https://github.com/chengsn/kite-desktop/releases/tag/` + tag + `",
		"body":"Release notes",
		"published_at":"2026-04-07T14:12:00Z",
		"assets":[
			{"name":"Kite-v1.2.4-macos-arm64.zip","browser_download_url":"https://example.com/macos-arm64.zip","content_type":"application/zip","size":2048},
			{"name":"Kite-v1.2.4-windows-amd64-installer.exe","browser_download_url":"https://example.com/windows-amd64-installer.exe","content_type":"application/octet-stream","size":4096}
		]
	}`
}

func resetUpdateCheckerState(t *testing.T) {
	t.Helper()

	origAPI := githubLatestReleaseAPI
	origClient := versionCheckClient
	origCachedUpdateResult := cachedUpdateResult
	origCachedUpdateKey := cachedUpdateKey
	origLastUpdateFetch := lastUpdateFetch

	githubLatestReleaseAPI = "https://example.com/releases/latest"
	cachedUpdateResult = updateCheckResult{}
	cachedUpdateKey = ""
	lastUpdateFetch = time.Time{}

	t.Cleanup(func() {
		githubLatestReleaseAPI = origAPI
		versionCheckClient = origClient
		cachedUpdateResult = origCachedUpdateResult
		cachedUpdateKey = origCachedUpdateKey
		lastUpdateFetch = origLastUpdateFetch
	})
}
