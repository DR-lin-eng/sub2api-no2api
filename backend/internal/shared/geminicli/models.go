package geminicli

// Model represents a selectable Gemini model for UI/testing purposes.
// Keep JSON fields consistent with existing frontend expectations.
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels is the curated Gemini model list used by the admin UI "test account" flow.
var DefaultModels = []Model{
	{ID: "gemini-4-argon", Type: "model", DisplayName: "Gemini 4 Argon"},
	{ID: "gemini-2.0-flash", Type: "model", DisplayName: "Gemini 2.0 Flash", CreatedAt: ""},
	{ID: "gemini-2.5-flash", Type: "model", DisplayName: "Gemini 2.5 Flash", CreatedAt: ""},
	{ID: "gemini-2.5-flash-image", Type: "model", DisplayName: "Gemini 2.5 Flash Image", CreatedAt: ""},
	{ID: "gemini-2.5-pro", Type: "model", DisplayName: "Gemini 2.5 Pro", CreatedAt: ""},
	{ID: "gemini-3.5-flash", Type: "model", DisplayName: "Gemini 3.5 Flash", CreatedAt: ""},
	{ID: "gemini-3-flash-preview", Type: "model", DisplayName: "Gemini 3 Flash Preview", CreatedAt: ""},
	{ID: "gemini-3-pro-preview", Type: "model", DisplayName: "Gemini 3 Pro Preview", CreatedAt: ""},
	{ID: "gemini-3.1-pro-preview", Type: "model", DisplayName: "Gemini 3.1 Pro Preview", CreatedAt: ""},
	{ID: "gemini-3.1-flash-image", Type: "model", DisplayName: "Gemini 3.1 Flash Image", CreatedAt: ""},
	{ID: "gemini-3.1-flash-lite-image", Type: "model", DisplayName: "Gemini 3.1 Flash Lite Image", CreatedAt: ""},
	{ID: "gemini-3.1-flash-image-preview", Type: "model", DisplayName: "Gemini 3.1 Flash Image Preview"},
	{ID: "gemini-3-pro-image", Type: "model", DisplayName: "Gemini 3 Pro Image"},
	{ID: "gemini-3-pro-image-preview", Type: "model", DisplayName: "Gemini 3 Pro Image Preview"},
	{ID: "gemini-3-flash", Type: "model", DisplayName: "Gemini 3 Flash"},
	{ID: "gemini-3.1-pro-preview-customtools", Type: "model", DisplayName: "Gemini 3.1 Pro Preview Customtools"},
	{ID: "gemini-3.1-flash-lite", Type: "model", DisplayName: "Gemini 3.1 Flash Lite"},
	{ID: "gemini-3.1-flash-lite-preview", Type: "model", DisplayName: "Gemini 3.1 Flash Lite Preview"},
	{ID: "gemini-3.5-flash-lite", Type: "model", DisplayName: "Gemini 3.5 Flash Lite"},
	{ID: "gemini-3.6-flash", Type: "model", DisplayName: "Gemini 3.6 Flash"},
	{ID: "gemini-3.7-flash", Type: "model", DisplayName: "Gemini 3.7 Flash"},
	{ID: "gemini-3.8-flash", Type: "model", DisplayName: "Gemini 3.8 Flash"},
}

// DefaultTestModel is the default model to preselect in test flows.
const DefaultTestModel = "gemini-3.5-flash"
