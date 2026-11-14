package openai

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Source: openai/codex 126a0ee92ed2c5e73cd8a8f7e37563e8527552d7.
//
//go:embed codex_gpt61_terra.json
var CodexGPT61TerraMetadata []byte

var codexTerraInstructions = func() string {
	var descriptor struct {
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	if err := json.Unmarshal(CodexGPT61TerraMetadata, &descriptor); err != nil {
		panic(err)
	}
	return descriptor.ModelMessages.InstructionsTemplate
}()

func IsGPT61TerraModelSpelling(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndexByte(model, '/'); slash >= 0 {
		model = model[slash+1:]
	}
	model = strings.ReplaceAll(model, "_", "-")
	if strings.HasPrefix(model, "gpt6") {
		model = "gpt-6" + strings.TrimPrefix(model, "gpt6")
	}
	if model == "gpt-6.1-terra" {
		return true
	}
	suffix, ok := strings.CutPrefix(model, "gpt-6.1-terra-")
	if !ok {
		return false
	}
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "openai-compact":
		return true
	}
	return false
}
