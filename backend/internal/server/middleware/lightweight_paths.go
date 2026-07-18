package middleware

const credentialKeyRequestPath = "/api/v1/auth/credential-key"

func isCredentialKeyRequestPath(path string) bool {
	return path == credentialKeyRequestPath
}
