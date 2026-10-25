package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
)

const (
	CallbackPath = "/auth/callback"
	httpTimeout  = 10 * time.Second
)

type Identity struct {
	Subject string
	Name    string
	Email   string
}

type OIDC struct {
	oauth      oauth2.Config
	client     *http.Client
	verifier   *oidc.IDTokenVerifier
	endSession string
	public     *url.URL
}

func New(ctx context.Context, cfg config.OIDC) (*OIDC, error) {
	public, err := url.Parse(strings.TrimSuffix(cfg.PublicURL, "/"))
	if err != nil || public.Host == "" || (public.Scheme != "https" && public.Scheme != "http") {
		return nil, fmt.Errorf("%s %q is not an http(s) URL", config.EnvPublicURL, cfg.PublicURL)
	}
	client := &http.Client{Timeout: httpTimeout}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, client), cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	var meta struct {
		EndSession string `json:"end_session_endpoint"`
	}
	if err := p.Claims(&meta); err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	return &OIDC{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  public.String() + CallbackPath,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		client:     client,
		verifier:   p.Verifier(&oidc.Config{ClientID: cfg.ClientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}}),
		endSession: meta.EndSession,
		public:     public,
	}, nil
}

func (o *OIDC) Secure() bool {
	return o.public.Scheme == "https"
}

func (o *OIDC) LogoutURL() string {
	return o.endSession
}

func (o *OIDC) AuthCodeURL(state, nonce, verifier string) string {
	return o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

func (o *OIDC) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	tok, err := o.oauth.Exchange(oidc.ClientContext(ctx, o.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, err
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return Identity{}, errors.New("token response has no id_token")
	}
	t, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	if t.Nonce != nonce {
		return Identity{}, errors.New("id token nonce does not match")
	}
	return identity(t)
}

func (o *OIDC) Verify(ctx context.Context, raw string) (Identity, error) {
	t, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	return identity(t)
}

func identity(t *oidc.IDToken) (Identity, error) {
	var c struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := t.Claims(&c); err != nil {
		return Identity{}, err
	}
	return Identity{Subject: t.Subject, Name: cmp.Or(c.Name, c.PreferredUsername, c.Email, t.Subject), Email: c.Email}, nil
}
