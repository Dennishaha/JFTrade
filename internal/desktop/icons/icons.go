package desktopicons

import _ "embed"

//go:embed jftrade-logo.png
var Application []byte

//go:embed jftrade-tray-light.png
var TrayLight []byte

//go:embed jftrade-tray-dark.png
var TrayDark []byte

//go:embed jftrade-tray-template.png
var TrayTemplate []byte
