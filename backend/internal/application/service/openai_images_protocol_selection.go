package service

// Select the native path before dispatch, so the App adapter cannot reject
// valid OpenAI Images parameters or discard them while building a conversation.
func openAIImagesRequiresNativeProtocol(req *OpenAIImagesRequest) bool {
	if req == nil {
		return false
	}
	// The App GIF adapter shares the account probe's conversation intent and
	// verifies the finished file's frame count/timing. Other native guarantees
	// still select the native path before any generation is sent.
	appGIF := usesCodexDirectImages(req.Model) && req.OutputFormat == "gif"
	return req.SVG != nil || !appGIF && (req.FrameCount != nil || req.FrameDurationMS != nil) ||
		req.HasMask || req.MaskUpload != nil || req.MaskImageURL != "" ||
		req.MarkupUpload != nil || req.MarkupImageURL != "" ||
		req.OutputCompression != nil || req.PartialImages != nil ||
		req.Complexity != "" || req.OutputFormat != "" && !appGIF ||
		req.Quality != "" && req.Quality != "auto" ||
		req.Background != "" && req.Background != "auto" ||
		req.ExplicitSize && req.Size != "auto" ||
		req.Style != "" || req.InputFidelity != "" || req.Moderation != "" ||
		req.SafetyIdentifier != "" || req.User != ""
}
