package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCToken struct {
	Issuer   string
	Subject  string
	Audience []string
	Expiry   time.Time
}

type OIDCUserInfo struct {
	Email string
	Name  string
}

type OIDCVerifier struct {
	issuer           string
	allowedAudiences map[string]struct{}
	provider         *oidc.Provider
	verifier         *oidc.IDTokenVerifier
	httpClient       *http.Client
	timeout          time.Duration
}

func NewOIDCVerifier(
	ctx context.Context,
	issuer string,
	allowedAudiences []string,
	httpClient *http.Client,
	timeout time.Duration,
) (*OIDCVerifier, error) {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		return nil, errors.New("missing OIDC issuer")
	}

	audSet := make(map[string]struct{}, len(allowedAudiences))
	for _, aud := range allowedAudiences {
		trimmed := strings.TrimSpace(aud)
		if trimmed == "" {
			continue
		}
		audSet[trimmed] = struct{}{}
	}
	if len(audSet) == 0 {
		return nil, errors.New("missing OIDC audiences")
	}

	if httpClient == nil {
		return nil, errors.New("missing OIDC http client")
	}
	if timeout <= 0 {
		return nil, errors.New("invalid OIDC timeout")
	}

	providerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	providerCtx = oidc.ClientContext(providerCtx, httpClient)

	provider, err := oidc.NewProvider(providerCtx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{SkipClientIDCheck: true})

	return &OIDCVerifier{
		issuer:           issuer,
		allowedAudiences: audSet,
		provider:         provider,
		verifier:         verifier,
		httpClient:       httpClient,
		timeout:          timeout,
	}, nil
}

func (v *OIDCVerifier) VerifyAccessToken(ctx context.Context, rawAccessToken string) (OIDCToken, error) {
	accessToken := strings.TrimSpace(rawAccessToken)
	if accessToken == "" {
		return OIDCToken{}, errors.New("missing access token")
	}

	verifyCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()
	verifyCtx = oidc.ClientContext(verifyCtx, v.httpClient)

	idToken, err := v.verifier.Verify(verifyCtx, accessToken)
	if err != nil {
		return OIDCToken{}, err
	}

	if !v.audienceAllowed(idToken.Audience) {
		return OIDCToken{}, errors.New("invalid audience")
	}

	return OIDCToken{
		Issuer:   idToken.Issuer,
		Subject:  idToken.Subject,
		Audience: append([]string(nil), idToken.Audience...),
		Expiry:   idToken.Expiry,
	}, nil
}

func (v *OIDCVerifier) UserInfo(ctx context.Context, rawAccessToken string) (OIDCUserInfo, error) {
	accessToken := strings.TrimSpace(rawAccessToken)
	if accessToken == "" {
		return OIDCUserInfo{}, errors.New("missing access token")
	}

	infoCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()
	infoCtx = oidc.ClientContext(infoCtx, v.httpClient)

	userInfo, err := v.provider.UserInfo(infoCtx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken}))
	if err != nil {
		return OIDCUserInfo{}, err
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := userInfo.Claims(&claims); err != nil {
		return OIDCUserInfo{}, err
	}

	return OIDCUserInfo{Email: claims.Email, Name: claims.Name}, nil
}

func (v *OIDCVerifier) audienceAllowed(audiences []string) bool {
	for _, aud := range audiences {
		if _, ok := v.allowedAudiences[aud]; ok {
			return true
		}
	}
	return false
}
