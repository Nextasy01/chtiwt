package auth

import "errors"

var (
	ErrUsernameTaken    = errors.New("username is taken")
	ErrEmailTaken       = errors.New("email is taken")
	ErrInvalidUsername  = errors.New("username must be 3-32 chars, letters/digits/underscore only")
	ErrReservedUsername = errors.New("username is reserved")
	ErrInvalidEmail     = errors.New("email is not valid")
	ErrInvalidPassword  = errors.New("password must be 8-72 chars")
	ErrBadCredentials   = errors.New("invalid username or password")
	ErrNoSession        = errors.New("no session")
	ErrNoChannel        = errors.New("no channel for user")
)
