package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"

	"github.com/jeremypoulter/google-sync/internal/config"
)

// Identity is who signed in, read from the ID token Google returns with the
// access token (scopes openid + email).
type Identity struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	// HostedDomain is the Workspace domain ("hd" claim); empty for consumer
	// accounts such as @gmail.com.
	HostedDomain string `json:"hd"`
}

// IdentityFromToken decodes the id_token returned alongside tok. The token
// came straight from Google's token endpoint over TLS, so its signature is
// not re-verified here.
func IdentityFromToken(tok *oauth2.Token) (*Identity, error) {
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return nil, errors.New("Google did not return an ID token (is the openid scope enabled?)")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed ID token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding ID token: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(payload, &id); err != nil {
		return nil, fmt.Errorf("decoding ID token: %w", err)
	}
	if id.Email == "" {
		return nil, errors.New("ID token has no email address")
	}
	return &id, nil
}

// CheckRole makes sure the account suits the role: the old account must be on
// a Workspace domain (wantDomain, if given), the new one must not be.
func CheckRole(r config.Role, id *Identity, wantDomain string) error {
	switch r {
	case config.RoleOld:
		if id.HostedDomain == "" {
			return fmt.Errorf("%s is not a Workspace account. Sign in with your OLD work/family-domain account here", id.Email)
		}
		if wantDomain != "" && !strings.EqualFold(id.HostedDomain, wantDomain) {
			return fmt.Errorf("%s is on %s, expected an account on %s", id.Email, id.HostedDomain, wantDomain)
		}
	case config.RoleNew:
		if id.HostedDomain != "" {
			return fmt.Errorf("%s is a Workspace account (%s). Sign in with your NEW personal Google account here", id.Email, id.HostedDomain)
		}
	default:
		return fmt.Errorf("unknown role %q", r)
	}
	return nil
}
