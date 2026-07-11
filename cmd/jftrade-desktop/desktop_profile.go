package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	desktopapp "github.com/jftrade/jftrade-main/internal/desktop"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const desktopDevelopmentAPIBind = "127.0.0.1:6698"

func developmentDesktopBuildProfile() desktopBuildProfile {
	return desktopBuildProfile{
		Channel:           "dev",
		ApplicationName:   "JFTrade Dev",
		ProductIdentifier: "com.jftrade.desktop.dev",
		SingleInstanceID:  "com.jftrade.desktop.dev",
		DefaultAPIBind:    desktopDevelopmentAPIBind,
	}
}

func releaseDesktopBuildProfile() desktopBuildProfile {
	return desktopBuildProfile{
		Channel:             "release",
		ApplicationName:     "JFTrade",
		ProductIdentifier:   "com.jftrade.desktop",
		SingleInstanceID:    "com.jftrade.desktop",
		DefaultAPIBind:      apiruntime.DefaultDesktopReleaseAPIBind,
		Release:             true,
		UpdateChecksEnabled: true,
	}
}

type desktopBuildProfile struct {
	Channel             string
	ApplicationName     string
	ProductIdentifier   string
	SingleInstanceID    string
	DefaultAPIBind      string
	Release             bool
	UpdateChecksEnabled bool
}

type desktopBootstrap struct {
	Profile   desktopBuildProfile
	Runtime   apiserver.DesktopRuntimeConfig
	StatePath string
}

func resolveDesktopBootstrap() (desktopBootstrap, error) {
	profile := currentDesktopBuildProfile()
	defaults, err := profile.launchDefaults()
	if err != nil {
		return desktopBootstrap{}, err
	}
	runtimeConfig, err := apiserver.ResolveDesktopRuntimeConfigWithDefaults(defaults, profile.Release)
	if err != nil {
		return desktopBootstrap{}, err
	}
	if profile.Release {
		runtimeConfig.APIToken, err = randomDesktopAPIToken()
		if err != nil {
			return desktopBootstrap{}, err
		}
	}
	return desktopBootstrap{
		Profile:   profile,
		Runtime:   runtimeConfig,
		StatePath: desktopWindowStatePath(profile, defaults.SettingsPath),
	}, nil
}

func randomDesktopAPIToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate desktop API token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (p desktopBuildProfile) launchDefaults() (jfsettings.LaunchDefaults, error) {
	if !p.Release {
		defaults := apiruntime.ResolveLaunchDefaults(false)
		defaults.APIBind = p.DefaultAPIBind
		return defaults, nil
	}

	root, err := desktopapp.ProductDataDir()
	if err != nil {
		return jfsettings.LaunchDefaults{}, err
	}
	if strings.TrimSpace(root) == "" {
		return jfsettings.LaunchDefaults{}, fmt.Errorf("packaged desktop data directory is empty")
	}
	return jfsettings.LaunchDefaults{
		APIBind:        p.DefaultAPIBind,
		SettingsPath:   filepath.Join(root, apiruntime.DefaultSettingsFilename),
		BacktestDBPath: filepath.Join(root, apiruntime.DefaultBacktestDBFilename),
	}, nil
}
