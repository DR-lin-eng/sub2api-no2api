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
	// Clients commonly send the documented defaults explicitly. They must not
	// move an App GIF edit onto a different endpoint that lacks GIF entitlement.
	complexityRequiresNative := req.Complexity != "" && !appGIF
	moderationRequiresNative := req.Moderation != "" && !appGIF
	qualityRequiresNative := req.Quality != "" && req.Quality != "auto" && !appGIF
	fidelityRequiresNative := req.InputFidelity != "" && !appGIF
	return req.SVG != nil || !appGIF && (req.FrameCount != nil || req.FrameDurationMS != nil) ||
		req.HasMask || req.MaskUpload != nil || req.MaskImageURL != "" ||
		req.MarkupUpload != nil || req.MarkupImageURL != "" ||
		req.OutputCompression != nil || req.PartialImages != nil ||
		complexityRequiresNative || req.OutputFormat != "" && !appGIF ||
		qualityRequiresNative ||
		req.Background != "" && req.Background != "auto" ||
		req.ExplicitSize && req.Size != "auto" ||
		req.Style != "" || fidelityRequiresNative || moderationRequiresNative ||
		req.SafetyIdentifier != "" || req.User != ""
}
