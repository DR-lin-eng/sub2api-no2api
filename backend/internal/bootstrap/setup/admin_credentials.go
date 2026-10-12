package setup

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin/binding"
)

const maxPasswordBytes = 72 // bcrypt's input limit

// validateEmail uses the login endpoint's rules so a new admin can log in.
func validateEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return false
	}
	login := struct {
		Email string `binding:"required,email"`
	}{Email: email}
	return binding.Validator.ValidateStruct(&login) == nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 bytes")
	}
	if len(password) > maxPasswordBytes {
		return fmt.Errorf("password must be at most %d bytes", maxPasswordBytes)
	}
	return nil
}

// prepareAdminCredentials runs only after deciding to create the first admin.
// Validate supplied values before generating or changing any credentials.
func prepareAdminCredentials(admin *AdminConfig) (emailGenerated, passwordGenerated bool, err error) {
	email := strings.TrimSpace(admin.Email)
	if email != "" && !validateEmail(email) {
		return false, false, fmt.Errorf("invalid admin email: address cannot be used to log in")
	}
	if strings.TrimSpace(admin.Password) != "" {
		if err := validatePassword(admin.Password); err != nil {
			return false, false, fmt.Errorf("invalid admin password: %w", err)
		}
	}
	if email == "" {
		email, err = generateAdminEmail()
		if err != nil {
			return false, false, err
		}
		emailGenerated = true
	}
	password := admin.Password
	if strings.TrimSpace(password) == "" {
		password, err = generateSecret(16)
		if err != nil {
			return false, false, fmt.Errorf("failed to generate admin password: %w", err)
		}
		passwordGenerated = true
	}
	admin.Email, admin.Password = email, password
	return emailGenerated, passwordGenerated, nil
}

func generateAdminEmail() (string, error) {
	suffix, err := generateSecret(6)
	if err != nil {
		return "", fmt.Errorf("failed to generate admin email: %w", err)
	}
	return "admin-" + suffix + "@sub2api.local", nil
}
