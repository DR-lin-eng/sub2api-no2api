package service

// Select the native path before dispatch, so the App adapter cannot reject
// valid OpenAI Images parameters or discard them while building a conversation.
func openAIImagesRequiresNativeProtocol(req *OpenAIImagesRequest) bool {
	if req == nil {
		return false
	}
	// Optional native controls that the recovered App contract cannot express
	// are deliberately ignored by the App adapter. Keep only options whose
	// omission would change the actual edit/output contract.
	return req.SVG != nil ||
		req.HasMask || req.MaskUpload != nil || req.MaskImageURL != "" ||
		req.MarkupUpload != nil || req.MarkupImageURL != "" ||
		req.OutputFormat == "svg"
}
