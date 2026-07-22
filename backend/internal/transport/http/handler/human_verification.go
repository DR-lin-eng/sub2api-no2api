package handler

import "strings"

func firstHumanVerificationToken(captchaToken, turnstileToken string) string {
	if token := strings.TrimSpace(captchaToken); token != "" {
		return token
	}
	return strings.TrimSpace(turnstileToken)
}
