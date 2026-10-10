package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileVerifiedTTL controls how long a successfully-verified token is trusted
// for follow-up requests in the same guest action (e.g. one photo-per-request upload batch).
const turnstileVerifiedTTL = 10 * time.Minute

// TurnstileGuard verifies Cloudflare Turnstile tokens with a short-lived success cache,
// since a Turnstile token can only be redeemed once against Cloudflare's siteverify API.
type TurnstileGuard struct {
	secretKey string
	client    *http.Client

	mu      sync.Mutex
	okUntil map[string]time.Time
}

// NewTurnstileGuard builds a guard. If secretKey is empty, verification is disabled
// (every token is accepted) so local development doesn't require live Cloudflare config.
func NewTurnstileGuard(secretKey string) *TurnstileGuard {
	return &TurnstileGuard{
		secretKey: secretKey,
		client:    &http.Client{Timeout: 5 * time.Second},
		okUntil:   make(map[string]time.Time),
	}
}

// Enabled reports whether real verification is configured.
func (g *TurnstileGuard) Enabled() bool {
	return g.secretKey != ""
}

// Verify checks a Turnstile response token, returning true if the guest passed the
// bot challenge. Previously-verified tokens are accepted again within the TTL window
// so a single widget solve can cover a multi-request upload batch.
func (g *TurnstileGuard) Verify(token, remoteIP string) bool {
	if !g.Enabled() {
		return true
	}
	if token == "" {
		return false
	}

	g.mu.Lock()
	if exp, ok := g.okUntil[token]; ok {
		if time.Now().Before(exp) {
			g.mu.Unlock()
			return true
		}
		delete(g.okUntil, token)
	}
	g.mu.Unlock()

	form := url.Values{}
	form.Set("secret", g.secretKey)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	resp, err := g.client.PostForm(turnstileVerifyURL, form)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	if result.Success {
		g.mu.Lock()
		g.okUntil[token] = time.Now().Add(turnstileVerifiedTTL)
		g.pruneLocked()
		g.mu.Unlock()
	}

	return result.Success
}

// pruneLocked drops expired cache entries. Caller must hold g.mu.
func (g *TurnstileGuard) pruneLocked() {
	now := time.Now()
	for tok, exp := range g.okUntil {
		if now.After(exp) {
			delete(g.okUntil, tok)
		}
	}
}
