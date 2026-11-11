package service

import "fmt"

func chatGPTAppGIFOptions(parsed *OpenAIImagesRequest) (int, int) {
	frames, delay := defaultOpenAIImagesGIFFrameCount, 200
	if parsed.FrameCount != nil {
		frames = *parsed.FrameCount
	}
	if parsed.FrameDurationMS != nil {
		delay = *parsed.FrameDurationMS
	}
	// The Images reference specifies nearest 10 ms, capped at the GIF field's
	// maximum. Clamp before addition to avoid overflowing a client integer.
	delay = (min(delay, 655350) + 5) / 10 * 10
	return frames, delay
}

func chatGPTAppImagePrompt(parsed *OpenAIImagesRequest, prompt string) string {
	if parsed.OutputFormat != "gif" {
		return prompt
	}
	frames, delay := chatGPTAppGIFOptions(parsed)
	intent := fmt.Sprintf("Create a short seamless looping animated GIF with exactly %d frames and %d milliseconds per frame. Return the finished GIF animation. Use the requested visual subject and motion. ", frames, delay)
	if parsed.IsEdits() {
		intent += "Animate the supplied reference images, preserving their subject and visual details. "
	}
	if parsed.Complexity != "" {
		intent += "Use " + parsed.Complexity + " planning complexity. "
	}
	if parsed.Quality != "" && parsed.Quality != "auto" {
		intent += "Render with " + parsed.Quality + " output quality. "
	}
	if parsed.Moderation != "" && parsed.Moderation != "auto" {
		intent += "Apply the requested " + parsed.Moderation + " moderation setting. "
	}
	if parsed.InputFidelity != "" {
		intent += "Preserve reference image details with " + parsed.InputFidelity + " input fidelity. "
	}
	if parsed.Style != "" {
		intent += "Use a " + parsed.Style + " visual style. "
	}
	return intent + prompt
}

// Conversation intent cannot guarantee native controls. Verify the file before
// returning success, and never relabel a PNG or silently ignore frame options.
func validateChatGPTAppImageOutput(parsed *OpenAIImagesRequest, image openAIResponsesImageResult) error {
	if parsed.OutputFormat != "gif" {
		return nil
	}
	if image.OutputFormat != "gif" {
		return fmt.Errorf("upstream did not return an animated GIF")
	}
	metadata, err := openAIImagesGIFMetadata(image.Result)
	if err != nil || metadata.FrameCount < 2 {
		return fmt.Errorf("upstream did not return a valid animated GIF file")
	}
	frames, delay := chatGPTAppGIFOptions(parsed)
	if metadata.FrameCount != frames {
		return fmt.Errorf("upstream GIF contains %d frames; %d were requested", metadata.FrameCount, frames)
	}
	for i := 0; i < metadata.FrameCount; i++ {
		if metadata.DelaysMS[i] != delay {
			return fmt.Errorf("upstream GIF frame %d has a %d ms delay; %d ms was requested", i+1, metadata.DelaysMS[i], delay)
		}
	}
	return nil
}
