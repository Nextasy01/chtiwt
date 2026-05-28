package auth

import (
	"net/mail"
	"regexp"
)

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

var reservedNames = map[string]bool{
	"admin": true, "root": true, "api": true, "static": true, "ws": true,
	"signup": true, "login": true, "logout": true, "dashboard": true,
	"healthz": true, "favicon": true, "c": true, "streams": true,
	"chtiwt": true, "system": true,
}

func validateUsername(s string) error {
	if !usernameRE.MatchString(s) {
		return ErrInvalidUsername
	}
	if reservedNames[s] {
		return ErrReservedUsername
	}
	return nil
}

func validateEmail(s string) error {
	if _, err := mail.ParseAddress(s); err != nil {
		return ErrInvalidEmail
	}
	return nil
}

func validatePassword(s string) error {
	if len(s) < 8 || len(s) > 72 {
		return ErrInvalidPassword
	}
	return nil
}
