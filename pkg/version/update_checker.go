package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	semver "github.com/blang/semver/v4"
	"k8s.io/klog/v2"
)

const (
	versionCheckTimeout = 3 * time.Second
	versionCacheTTL     = time.Hour
)

var githubLatestReleaseAPI = "https://api.github.com/repos/chengsn/kite-desktop/releases/latest"

const (
	UpdateSourceAuto   = "auto"
	UpdateSourceGitHub = "github"
	UpdateSourceCNB    = "cnb"
)

var (
	updateInfoMu       sync.Mutex
	cachedUpdateResult = updateCheckResult{}
	cachedUpdateKey    string
	lastUpdateFetch    time.Time
	versionCheckClient = http.DefaultClient
)

type UpdateComparison string

const (
	UpdateComparisonUpdateAvailable UpdateComparison = "update_available"
	UpdateComparisonUpToDate        UpdateComparison = "up_to_date"
	UpdateComparisonLocalNewer      UpdateComparison = "local_newer"
	UpdateComparisonUncomparable    UpdateComparison = "uncomparable"
)

type UpdateAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"downloadUrl"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type githubRelease struct {
	TagName     string               `json:"tag_name"`
	HTMLURL     string               `json:"html_url"`
	Body        string               `json:"body"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
}

type updateCheckResult struct {
	comparison     UpdateComparison
	latestVersion  string
	releaseURL     string
	releaseNotes   string
	publishedAt    string
	assetAvailable bool
	asset          *UpdateAsset
	checkedAt      time.Time
}

func checkForUpdate(ctx context.Context, currentVersion string, force bool, updateSource string) (updateCheckResult, error) {
	sanitized := strings.TrimSpace(currentVersion)
	if sanitized == "" || strings.EqualFold(sanitized, "dev") {
		return updateCheckResult{
			comparison: UpdateComparisonUncomparable,
			checkedAt:  time.Now(),
		}, nil
	}
	source := ResolvePreferredUpdateSource(updateSource)
	cacheKey := buildUpdateCacheKey(sanitized, source)

	updateInfoMu.Lock()
	if !force && time.Since(lastUpdateFetch) < versionCacheTTL && cachedUpdateKey == cacheKey {
		cached := cachedUpdateResult
		updateInfoMu.Unlock()
		return cached, nil
	}
	updateInfoMu.Unlock()

	requestCtx, cancel := context.WithTimeout(ctx, versionCheckTimeout)
	defer cancel()

	result := updateCheckResult{
		comparison: UpdateComparisonUncomparable,
		checkedAt:  time.Now(),
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, githubLatestReleaseAPI, nil)
	if err != nil {
		klog.Warningf("version check request creation failed: %v", err)
		return result, fmt.Errorf("create version check request: %w", err)
	}

	req.Header.Set("User-Agent", "kite-version-checker/"+currentVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := versionCheckClient.Do(req)
	if err != nil {
		klog.Warningf("version check request failed: %v", err)
		return result, fmt.Errorf("request latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		klog.Warningf("version check unexpected status: %s", resp.Status)
		return result, fmt.Errorf("unexpected latest release status: %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		klog.Warningf("version check decode failed: %v", err)
		return result, fmt.Errorf("decode latest release response: %w", err)
	}

	result.latestVersion = normalizeVersionValue(release.TagName)
	result.releaseURL = release.HTMLURL
	result.releaseNotes = release.Body
	result.publishedAt = normalizePublishedAt(release.PublishedAt)
	result.asset = matchReleaseAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	result.assetAvailable = result.asset != nil

	latestVersion, err := parseSemver(release.TagName)
	if err != nil {
		klog.Warningf("latest version parse failed, marking update state as uncomparable: %v", err)
		cacheUpdateResult(cacheKey, result)
		return result, nil
	}
	result.latestVersion = latestVersion.String()

	currentSemver, err := parseSemver(sanitized)
	if err != nil {
		klog.Warningf("current version parse failed, marking update state as uncomparable: %v", err)
		cacheUpdateResult(cacheKey, result)
		return result, nil
	}

	switch {
	case latestVersion.GT(currentSemver):
		result.comparison = UpdateComparisonUpdateAvailable
	case latestVersion.EQ(currentSemver):
		result.comparison = UpdateComparisonUpToDate
	default:
		result.comparison = UpdateComparisonLocalNewer
	}
	applyUpdateSource(&result, source)

	cacheUpdateResult(cacheKey, result)
	return result, nil
}

func cacheUpdateResult(cacheKey string, result updateCheckResult) {
	updateInfoMu.Lock()
	cachedUpdateResult = result
	cachedUpdateKey = cacheKey
	lastUpdateFetch = time.Now()
	updateInfoMu.Unlock()
}

func parseSemver(version string) (semver.Version, error) {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if trimmed == "" {
		return semver.Version{}, errors.New("empty version")
	}

	parsed, err := semver.Parse(trimmed)
	if err != nil {
		return semver.Version{}, fmt.Errorf("invalid semver %q: %w", version, err)
	}
	return parsed, nil
}

func buildUpdateCacheKey(version, source string) string {
	return fmt.Sprintf("%s|%s|%s|%s", normalizeVersionValue(version), runtime.GOOS, runtime.GOARCH, ResolvePreferredUpdateSource(source))
}

func normalizeVersionValue(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func normalizePublishedAt(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return trimmed
	}
	return parsed.Format(time.RFC3339)
}

func matchReleaseAsset(assets []githubReleaseAsset, goos, goarch string) *UpdateAsset {
	if goos != "darwin" && goos != "windows" {
		return nil
	}

	preferredExts := []string{".zip"}
	switch goos {
	case "windows":
		preferredExts = []string{"-installer.exe"}
	case "darwin":
		preferredExts = []string{".zip", ".dmg"}
	}

	for _, ext := range preferredExts {
		for _, asset := range assets {
			if !releaseAssetMatches(asset, goos, goarch, ext) {
				continue
			}
			return &UpdateAsset{
				Name:        asset.Name,
				DownloadURL: asset.BrowserDownloadURL,
				ContentType: asset.ContentType,
				Size:        asset.Size,
			}
		}
	}

	return nil
}

func releaseAssetMatches(asset githubReleaseAsset, goos, goarch, suffix string) bool {
	name := strings.ToLower(strings.TrimSpace(asset.Name))
	if name == "" || !strings.HasSuffix(name, suffix) {
		return false
	}

	osTokens := map[string][]string{
		"darwin":  {"macos", "darwin"},
		"windows": {"windows"},
	}
	archTokens := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64", "intel"},
		"arm64": {"arm64", "aarch64", "apple-silicon"},
	}

	if !containsAnyToken(name, osTokens[goos]) {
		return false
	}
	return containsAnyToken(name, archTokens[goarch])
}

func containsAnyToken(value string, tokens []string) bool {
	return slices.ContainsFunc(tokens, func(token string) bool {
		return strings.Contains(value, token)
	})
}

func NormalizeUpdateSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case UpdateSourceGitHub:
		return UpdateSourceGitHub
	case UpdateSourceCNB:
		return UpdateSourceCNB
	default:
		return UpdateSourceAuto
	}
}

func ResolvePreferredUpdateSource(source string) string {
	normalized := NormalizeUpdateSource(source)
	if normalized == UpdateSourceAuto {
		return UpdateSourceGitHub
	}
	return normalized
}

func RewriteReleaseURLBySource(rawURL, source string) string {
	switch ResolvePreferredUpdateSource(source) {
	case UpdateSourceCNB:
		return githubURLToCNB(rawURL)
	case UpdateSourceGitHub:
		return cnbURLToGitHub(rawURL)
	default:
		return rawURL
	}
}

func RewriteAssetDownloadURLBySource(rawURL, source string) string {
	return RewriteReleaseURLBySource(rawURL, source)
}

func AlternateMirrorDownloadURL(rawURL string) string {
	if rewritten := githubURLToCNB(rawURL); rewritten != rawURL {
		return rewritten
	}
	if rewritten := cnbURLToGitHub(rawURL); rewritten != rawURL {
		return rewritten
	}
	return ""
}

func applyUpdateSource(result *updateCheckResult, source string) {
	if result == nil {
		return
	}
	result.releaseURL = RewriteReleaseURLBySource(result.releaseURL, source)
	if result.asset == nil {
		return
	}
	asset := *result.asset
	asset.DownloadURL = RewriteAssetDownloadURLBySource(asset.DownloadURL, source)
	result.asset = &asset
}

func githubURLToCNB(rawURL string) string {
	const githubPrefix = "https://github.com/chengsn/kite-desktop/"
	if !strings.HasPrefix(rawURL, githubPrefix) {
		return rawURL
	}
	rest := strings.TrimPrefix(rawURL, githubPrefix)
	if !strings.HasPrefix(rest, "releases/") {
		return rawURL
	}
	return "https://cnb.cool/chengsn/kite-desktop/-/" + rest
}

func cnbURLToGitHub(rawURL string) string {
	const cnbPrefix = "https://cnb.cool/chengsn/kite-desktop/-/"
	if !strings.HasPrefix(rawURL, cnbPrefix) {
		return rawURL
	}
	rest := strings.TrimPrefix(rawURL, cnbPrefix)
	if !strings.HasPrefix(rest, "releases/") {
		return rawURL
	}
	return "https://github.com/chengsn/kite-desktop/" + rest
}
