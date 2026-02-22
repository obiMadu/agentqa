package httpapi

import (
  "context"
  "encoding/json"
  "errors"
  "net/http"
  "net/url"
)

type googleTokenInfo struct {
  Email string `json:"email"`
  Name  string `json:"name"`
  Aud   string `json:"aud"`
}

func (s *Server) verifyGoogleIDToken(ctx context.Context, idToken string) (googleTokenInfo, error) {
  if s.cfg.GoogleClientID == "" {
    return googleTokenInfo{}, errors.New("missing google client id")
  }

  endpoint := "https://oauth2.googleapis.com/tokeninfo"
  params := url.Values{}
  params.Set("id_token", idToken)

  req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
  if err != nil {
    return googleTokenInfo{}, err
  }

  resp, err := http.DefaultClient.Do(req)
  if err != nil {
    return googleTokenInfo{}, err
  }
  defer resp.Body.Close()

  if resp.StatusCode != http.StatusOK {
    return googleTokenInfo{}, errors.New("tokeninfo failed")
  }

  var info googleTokenInfo
  if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
    return googleTokenInfo{}, err
  }

  if info.Aud != s.cfg.GoogleClientID {
    return googleTokenInfo{}, errors.New("invalid audience")
  }

  if info.Email == "" {
    return googleTokenInfo{}, errors.New("missing email")
  }

  return info, nil
}
