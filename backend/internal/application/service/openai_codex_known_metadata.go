package service

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

var codexTerraMetadata = func() modelcatalog.Descriptor {
	var descriptor modelcatalog.Descriptor
	if err := json.Unmarshal(openai.CodexGPT61TerraMetadata, &descriptor); err != nil {
		panic(err)
	}
	return descriptor
}()

// Return a request-owned descriptor; upstream presence and administrator overrides still win.
func codexKnownModelFallback(id string) modelcatalog.Descriptor {
	fallback := modelcatalog.KnownFallback(id)
	if openai.IsGPT61TerraModelSpelling(id) {
		return modelcatalog.Fill(codexTerraMetadata, fallback)
	}
	return fallback
}
