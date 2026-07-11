package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func readAuthDocument(path string) (*authDocument, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("codex: read credentials %q: %w", path, err)
	}

	root := make(map[string]json.RawMessage)
	if err := json.Unmarshal(contents, &root); err != nil {
		return nil, fmt.Errorf("codex: decode credentials %q: %w", path, err)
	}
	document := &authDocument{raw: root}
	_ = json.Unmarshal(root["auth_mode"], &document.authMode)

	tokenRaw := make(map[string]json.RawMessage)
	if len(root["tokens"]) == 0 || string(root["tokens"]) == "null" {
		return nil, ErrSubscriptionRequired
	}
	if err := json.Unmarshal(root["tokens"], &tokenRaw); err != nil {
		return nil, fmt.Errorf("codex: decode credential tokens: %w", err)
	}
	document.tokens.raw = tokenRaw
	_ = json.Unmarshal(tokenRaw["id_token"], &document.tokens.idToken)
	_ = json.Unmarshal(tokenRaw["access_token"], &document.tokens.accessToken)
	_ = json.Unmarshal(tokenRaw["refresh_token"], &document.tokens.refreshToken)
	_ = json.Unmarshal(tokenRaw["account_id"], &document.tokens.accountID)
	return document, nil
}

func (d *authDocument) credential() (credential, error) {
	mode := strings.ToLower(strings.TrimSpace(d.authMode))
	if mode != "" && mode != "chatgpt" {
		return credential{}, ErrSubscriptionRequired
	}
	if strings.TrimSpace(d.tokens.accessToken) == "" {
		return credential{}, ErrNotAuthenticated
	}

	claims, _ := parseJWTClaims(d.tokens.idToken)
	if claims.Auth.AccountID == "" {
		if accessClaims, err := parseJWTClaims(d.tokens.accessToken); err == nil {
			claims.Auth = accessClaims.Auth
		}
	}
	accountID := d.tokens.accountID
	if accountID == "" {
		accountID = claims.Auth.AccountID
	}
	if accountID == "" {
		return credential{}, errors.New("codex: ChatGPT account id is missing from CLI credentials")
	}
	return credential{
		accessToken: d.tokens.accessToken, accountID: accountID,
		plan: claims.Auth.Plan, fedramp: claims.Auth.FedRAMP,
	}, nil
}

func writeAuthDocument(path string, document *authDocument, now time.Time) error {
	setToken := func(name, value string) error {
		encoded, err := json.Marshal(value)
		if err == nil {
			document.tokens.raw[name] = encoded
		}
		return err
	}
	if err := setToken("id_token", document.tokens.idToken); err != nil {
		return err
	}
	if err := setToken("access_token", document.tokens.accessToken); err != nil {
		return err
	}
	if err := setToken("refresh_token", document.tokens.refreshToken); err != nil {
		return err
	}
	if err := setToken("account_id", document.tokens.accountID); err != nil {
		return err
	}
	tokens, err := json.Marshal(document.tokens.raw)
	if err != nil {
		return fmt.Errorf("codex: encode refreshed tokens: %w", err)
	}
	document.raw["tokens"] = tokens
	lastRefresh, _ := json.Marshal(now.UTC().Format(time.RFC3339Nano))
	document.raw["last_refresh"] = lastRefresh

	contents, err := json.MarshalIndent(document.raw, "", "  ")
	if err != nil {
		return fmt.Errorf("codex: encode refreshed credentials: %w", err)
	}
	contents = append(contents, '\n')

	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".auth.json.dire-agent-*")
	if err != nil {
		return fmt.Errorf("codex: create temporary credential file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("codex: secure temporary credential file: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("codex: write refreshed credentials: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("codex: sync refreshed credentials: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("codex: close refreshed credentials: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("codex: replace refreshed credentials: %w", err)
	}
	return nil
}

func expiresSoon(token string, now time.Time, window time.Duration) bool {
	claims, err := parseJWTClaims(token)
	if err != nil || claims.ExpiresAt == 0 {
		return false
	}
	return time.Unix(claims.ExpiresAt, 0).Before(now.Add(window))
}

func parseJWTClaims(token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, errors.New("invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, err
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenClaims{}, err
	}
	return claims, nil
}
