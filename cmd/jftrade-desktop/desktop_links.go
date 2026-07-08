package main

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	desktopOpenLinkMethodID = 0x4a465401
	desktopDocsWindowName   = "docs"
)

func init() {
	application.RegisterBindingMethodID((*DesktopLinkService).OpenLink, desktopOpenLinkMethodID)
}

type DesktopLinkService struct {
	app *application.App
}

func (s *DesktopLinkService) OpenLink(rawLink string) error {
	link, err := classifyDesktopLink(rawLink)
	if err != nil {
		return err
	}
	if link.externalURL != "" {
		if s == nil || s.app == nil {
			return fmt.Errorf("desktop application is not ready")
		}
		return s.app.Browser.OpenURL(link.externalURL)
	}
	if link.docsURL != "" {
		if s == nil || s.app == nil {
			return fmt.Errorf("desktop application is not ready")
		}
		s.openDocsWindow(link.docsURL)
		return nil
	}
	return fmt.Errorf("unsupported desktop link")
}

func (s *DesktopLinkService) openDocsWindow(docsURL string) {
	if window, ok := s.app.Window.GetByName(desktopDocsWindowName); ok {
		window.SetURL(docsURL).Show().Focus()
		return
	}

	window := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            desktopDocsWindowName,
		Title:           "JFTrade 文档",
		URL:             docsURL,
		Width:           1120,
		Height:          760,
		MinWidth:        820,
		MinHeight:       600,
		InitialPosition: application.WindowCentered,
		Zoom:            desktopWebviewZoom,
	})
	window.SetZoom(desktopWebviewZoom)
	window.Show().Focus()
}

type desktopLinkTarget struct {
	docsURL     string
	externalURL string
}

func classifyDesktopLink(rawLink string) (desktopLinkTarget, error) {
	if externalURL, ok, err := sanitizeDesktopExternalURL(rawLink); ok || err != nil {
		return desktopLinkTarget{externalURL: externalURL}, err
	}
	docsURL, err := normalizeDesktopDocsURL(rawLink)
	if err != nil {
		return desktopLinkTarget{}, err
	}
	return desktopLinkTarget{docsURL: docsURL}, nil
}

func sanitizeDesktopExternalURL(rawLink string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawLink)
	if trimmed == "" {
		return "", false, fmt.Errorf("link is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", false, fmt.Errorf("invalid link: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return "", false, nil
	}
	if scheme != "http" && scheme != "https" {
		return "", true, fmt.Errorf("unsupported link scheme %q", scheme)
	}
	sanitized, err := application.ValidateAndSanitizeURL(trimmed)
	if err != nil {
		return "", true, err
	}
	return sanitized, true, nil
}

func normalizeDesktopDocsURL(rawLink string) (string, error) {
	trimmed := strings.TrimSpace(rawLink)
	if trimmed == "" {
		return "", fmt.Errorf("link is empty")
	}
	if hasUnsafeDesktopLinkCharacter(trimmed) {
		return "", fmt.Errorf("link contains unsafe characters")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid docs link: %w", err)
	}
	if parsed.Scheme != "" || parsed.Host != "" {
		return "", fmt.Errorf("unsupported docs link")
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("docs link path is empty")
	}

	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == ".." {
			return "", fmt.Errorf("docs link must not contain parent traversal")
		}
	}

	cleanPath := path.Clean("/" + strings.TrimPrefix(parsed.Path, "/"))
	if cleanPath == "." || cleanPath == "/" || cleanPath == "/docs" {
		cleanPath = "/docs/"
	}
	if cleanPath != "/docs" && !strings.HasPrefix(cleanPath, "/docs/") {
		return "", fmt.Errorf("docs link must start with /docs/")
	}
	if strings.HasSuffix(cleanPath, "/index.html") {
		cleanPath = strings.TrimSuffix(cleanPath, "index.html")
	}

	docsURL := url.URL{
		Path:     cleanPath,
		RawQuery: parsed.RawQuery,
		Fragment: parsed.Fragment,
	}
	return docsURL.String(), nil
}

func hasUnsafeDesktopLinkCharacter(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
