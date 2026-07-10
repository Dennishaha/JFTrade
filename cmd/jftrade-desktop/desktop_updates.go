package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/mod/semver"

	"github.com/jftrade/jftrade-main/internal/buildinfo"
)

const (
	desktopReleaseRepository = "Dennishaha/jftrade"
	desktopReleaseTagPrefix  = "desktop-v"
	desktopUpdateEvent       = "jftrade:desktop-update:available"
)

type DesktopUpdateResult struct {
	CurrentVersion string `json:"currentVersion"`
	Available      bool   `json:"available"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type githubDesktopRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// DesktopUpdateService checks the public desktop release feed without
// downloading or installing application updates.
type DesktopUpdateService struct {
	enabled     bool
	current     string
	releasesURL string
	client      *http.Client
}

func newDesktopUpdateService(profile desktopBuildProfile) *DesktopUpdateService {
	return &DesktopUpdateService{
		enabled:     profile.UpdateChecksEnabled,
		current:     normalizeDesktopVersion(buildinfo.Version),
		releasesURL: "https://api.github.com/repos/" + desktopReleaseRepository + "/releases?per_page=20",
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *DesktopUpdateService) Check() (DesktopUpdateResult, error) {
	if s == nil {
		return DesktopUpdateResult{CurrentVersion: "dev"}, nil
	}
	current := normalizeDesktopVersion(s.current)
	result := DesktopUpdateResult{CurrentVersion: current}
	if !s.enabled || current == "" || current == "dev" {
		return result, nil
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.releasesURL, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "JFTrade-Desktop/"+current)
	resp, err := s.client.Do(req)
	if err != nil {
		return result, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("desktop release check returned HTTP %d", resp.StatusCode)
	}
	var releases []githubDesktopRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return result, fmt.Errorf("decode desktop release feed: %w", err)
	}
	latest := latestDesktopRelease(releases)
	if latest == nil {
		return result, nil
	}
	result.LatestVersion = normalizeDesktopVersion(latest.TagName)
	result.ReleaseURL = strings.TrimSpace(latest.HTMLURL)
	result.PublishedAt = strings.TrimSpace(latest.PublishedAt)
	result.Notes = strings.TrimSpace(latest.Body)
	result.Available = semver.Compare("v"+result.LatestVersion, "v"+current) > 0
	return result, nil
}

func normalizeDesktopVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, desktopReleaseTagPrefix)
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return "dev"
	}
	return value
}

func latestDesktopRelease(releases []githubDesktopRelease) *githubDesktopRelease {
	var latest *githubDesktopRelease
	latestVersion := ""
	for index := range releases {
		release := &releases[index]
		if release.Draft || release.Prerelease || !strings.HasPrefix(release.TagName, desktopReleaseTagPrefix) {
			continue
		}
		version := normalizeDesktopVersion(release.TagName)
		if !semver.IsValid("v" + version) {
			continue
		}
		if latest == nil || semver.Compare("v"+version, "v"+latestVersion) > 0 {
			latest = release
			latestVersion = version
		}
	}
	return latest
}

func startDesktopUpdateChecks(ctx context.Context, app *application.App, service *DesktopUpdateService) {
	if ctx == nil || app == nil || service == nil || !service.enabled {
		return
	}
	go func() {
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			result, err := service.Check()
			if err != nil {
				app.Logger.Warn("desktop update check failed", "error", err)
			} else if result.Available {
				app.Event.Emit(desktopUpdateEvent, result)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func checkDesktopUpdateInteractively(window application.Window, app *application.App, service *DesktopUpdateService) {
	if window == nil || app == nil || service == nil {
		return
	}
	go func() {
		result, err := service.Check()
		if err != nil {
			window.Error("检查更新失败: %v", err)
			return
		}
		if !result.Available {
			window.Info("当前已是最新版本 (%s)", result.CurrentVersion)
			return
		}
		if result.ReleaseURL != "" {
			jftradeLogError(app.Browser.OpenURL(result.ReleaseURL))
		}
	}()
}
