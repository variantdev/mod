// This file contains code derived from heroku/docker-registry-client.
// Original source: https://github.com/heroku/docker-registry-client
//
// The following types and functions are derived from:
// - TokenTransport, authToken, authService: https://github.com/heroku/docker-registry-client/blob/master/registry/tokentransport.go
// - AuthorizationChallenge, parseAuthHeader, parseValueAndParams, and related functions: https://github.com/heroku/docker-registry-client/blob/master/registry/authchallenge.go

package dockerregistry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// TokenTransport is an http.RoundTripper that handles Docker Registry token authentication.
// When a request receives a 401 with a WWW-Authenticate header, it fetches a token
// from the auth service and retries the request with the bearer token.
type TokenTransport struct {
	Transport http.RoundTripper
	Username  string
	Password  string
}

func (t *TokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if authService := isTokenDemand(resp); authService != nil {
		resp.Body.Close()
		resp, err = t.authAndRetry(authService, req)
	}
	return resp, err
}

type authToken struct {
	Token string `json:"token"`
}

func (t *TokenTransport) authAndRetry(authService *authService, req *http.Request) (*http.Response, error) {
	token, authResp, err := t.auth(authService)
	if err != nil {
		return authResp, err
	}

	return t.retry(req, token)
}

func (t *TokenTransport) auth(authService *authService) (string, *http.Response, error) {
	authReq, err := authService.Request(t.Username, t.Password)
	if err != nil {
		return "", nil, err
	}

	client := http.Client{
		Transport: t.Transport,
	}

	response, err := client.Do(authReq)
	if err != nil {
		return "", nil, err
	}

	if response.StatusCode != http.StatusOK {
		return "", response, fmt.Errorf("auth failed with status: %d", response.StatusCode)
	}
	defer response.Body.Close()

	var token authToken
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&token); err != nil {
		return "", nil, err
	}

	return token.Token, nil, nil
}

func (t *TokenTransport) retry(req *http.Request, token string) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return t.Transport.RoundTrip(req)
}

type authService struct {
	Realm   string
	Service string
	Scope   string
}

func (a *authService) Request(username, password string) (*http.Request, error) {
	u, err := url.Parse(a.Realm)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("service", a.Service)
	if a.Scope != "" {
		q.Set("scope", a.Scope)
	}
	u.RawQuery = q.Encode()

	request, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	if username != "" || password != "" {
		request.SetBasicAuth(username, password)
	}

	return request, nil
}

func isTokenDemand(resp *http.Response) *authService {
	if resp == nil {
		return nil
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return nil
	}
	return parseOauthHeader(resp)
}

func parseOauthHeader(resp *http.Response) *authService {
	challenges := parseAuthHeader(resp.Header)
	for _, challenge := range challenges {
		if challenge.Scheme == "bearer" {
			return &authService{
				Realm:   challenge.Parameters["realm"],
				Service: challenge.Parameters["service"],
				Scope:   challenge.Parameters["scope"],
			}
		}
	}
	return nil
}

// AuthorizationChallenge carries information from a WWW-Authenticate response header.
type AuthorizationChallenge struct {
	Scheme     string
	Parameters map[string]string
}

// Octet types from RFC 2616.
type octetType byte

const (
	isToken octetType = 1 << iota
	isSpace
)

var octetTypes [256]octetType

func init() {
	for c := 0; c < 256; c++ {
		var t octetType
		isCtl := c <= 31 || c == 127
		isChar := 0 <= c && c <= 127
		isSeparator := strings.ContainsRune(" \t\"(),/:;<=>?@[]\\{}", rune(c))
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			t |= isSpace
		}
		if isChar && !isCtl && !isSeparator {
			t |= isToken
		}
		octetTypes[c] = t
	}
}

func parseAuthHeader(header http.Header) []*AuthorizationChallenge {
	var challenges []*AuthorizationChallenge
	for _, h := range header[http.CanonicalHeaderKey("WWW-Authenticate")] {
		v, p := parseValueAndParams(h)
		if v != "" {
			challenges = append(challenges, &AuthorizationChallenge{Scheme: v, Parameters: p})
		}
	}
	return challenges
}

func parseValueAndParams(header string) (value string, params map[string]string) {
	params = make(map[string]string)
	value, s := expectToken(header)
	if value == "" {
		return
	}
	value = strings.ToLower(value)
	s = "," + skipSpace(s)
	for strings.HasPrefix(s, ",") {
		var pkey string
		pkey, s = expectToken(skipSpace(s[1:]))
		if pkey == "" {
			return
		}
		if !strings.HasPrefix(s, "=") {
			return
		}
		var pvalue string
		pvalue, s = expectTokenOrQuoted(s[1:])
		if pvalue == "" {
			return
		}
		pkey = strings.ToLower(pkey)
		params[pkey] = pvalue
		s = skipSpace(s)
	}
	return
}

func skipSpace(s string) string {
	i := 0
	for ; i < len(s); i++ {
		if octetTypes[s[i]]&isSpace == 0 {
			break
		}
	}
	return s[i:]
}

func expectToken(s string) (token, rest string) {
	i := 0
	for ; i < len(s); i++ {
		if octetTypes[s[i]]&isToken == 0 {
			break
		}
	}
	return s[:i], s[i:]
}

func expectTokenOrQuoted(s string) (value string, rest string) {
	if !strings.HasPrefix(s, "\"") {
		return expectToken(s)
	}
	s = s[1:]
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			return s[:i], s[i+1:]
		case '\\':
			p := make([]byte, len(s)-1)
			j := copy(p, s[:i])
			escape := true
			for i++; i < len(s); i++ {
				b := s[i]
				switch {
				case escape:
					escape = false
					p[j] = b
					j++
				case b == '\\':
					escape = true
				case b == '"':
					return string(p[:j]), s[i+1:]
				default:
					p[j] = b
					j++
				}
			}
			return "", ""
		}
	}
	return "", ""
}

// WrapTransport wraps an http.RoundTripper with token authentication support.
func WrapTransport(transport http.RoundTripper, username, password string) http.RoundTripper {
	return &TokenTransport{
		Transport: transport,
		Username:  username,
		Password:  password,
	}
}
