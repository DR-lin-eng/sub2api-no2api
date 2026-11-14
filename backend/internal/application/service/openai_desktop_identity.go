package service

import (
	"fmt"
	"net/http"
	"runtime"
)

// This is the Electron App version from the 20261115 desktop report. It is
// independent of the Rust engine version used for Codex model requests.
const openAIChatGPTDesktopVersion = "26.1111.21308"

func applyOpenAIChatGPTDesktopSurfaceHeaders(headers http.Header) {
	headers.Set("User-Agent", fmt.Sprintf("Codex Desktop/%s (%s; %s)", openAIChatGPTDesktopVersion, runtime.GOOS, runtime.GOARCH))
	headers.Set("originator", "Codex Desktop")
	headers.Set("x-openai-web-frontend", "codex_webview")
	headers.Set("x-openai-codex-window-type", "electron")
}
