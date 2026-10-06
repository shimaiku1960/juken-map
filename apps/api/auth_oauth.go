package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
)

// 外部ログイン（Google・GitHub、認証基準 10 の F1〜F3、06 の C4）。
//
// 往復の流れ
//  1. 画面が POST /api/auth/oauth/{provider} を呼ぶ。state・PKCE の code_verifier・nonce を作って DB に置き、
//     state は Cookie にも置いて、プロバイダーの同意画面の URL を返す
//  2. 利用者が同意すると、プロバイダーが GET /api/auth/callback/{provider}?code&state へ戻す
//  3. Cookie の state と URL の state が合い、DB にもあるときだけ続ける（06 C4。別の人の往復を差し込ませない）
//  4. code と code_verifier でトークンに替える（PKCE：途中で code を盗んでも、verifier が無ければ替えられない）
//  5. Google は ID トークンの iss・aud・exp・nonce を確かめる。GitHub は API で ID とメールを引く
//  6. 利用者を決めて（F2）、ログインを完了する（2段階認証が有効なら、その途中の状態へ）
//
// 戻り先の URL（redirect_uri）は「自分のオリジン + /api/auth/callback/{provider}」に固定する。
// Google・GitHub の設定に登録してある URL と同じ（Better Auth と同じパスなので、設定は変えない）。

// state の寿命（cookie と DB の行）は authguard.OAuthStateTTL。保存と消費は持ち主の internal/write/authguard（JUK-154）。

var errOAuthEmailNotVerified = errors.New("プロバイダーがメールアドレスを確認済みと示していない")

// oauthIdentity はプロバイダーから受け取った利用者。
type oauthIdentity struct {
	Subject       string // Google の sub、GitHub の数値の ID
	Email         string
	EmailVerified bool
	Name          string
	Image         string
}

type oauthProvider struct {
	name   string
	config *oauth2.Config
	// usesNonce は OIDC（ID トークン）を使うか。Google だけ。
	usesNonce bool
	identify  func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token, nonce string, now time.Time) (*oauthIdentity, error)
}

// oauthEndpoints はプロバイダーの URL。テストでは偽のサーバーへ向ける。
type oauthEndpoints struct {
	googleAuth, googleToken            string
	githubAuth, githubToken, githubAPI string
}

var defaultOAuthEndpoints = oauthEndpoints{
	googleAuth:  "https://accounts.google.com/o/oauth2/v2/auth",
	googleToken: "https://oauth2.googleapis.com/token",
	githubAuth:  "https://github.com/login/oauth/authorize",
	githubToken: "https://github.com/login/oauth/access_token",
	githubAPI:   "https://api.github.com",
}

// newOAuthProviders は設定のあるプロバイダーだけを作る。scope はログインに要る最小限（ID とメール）にする（F3）。
// 表示名や画像のための profile（Google）・read:user（GitHub）は求めない。
func newOAuthProviders(webOrigin string, ep oauthEndpoints, googleID, googleSecret, githubID, githubSecret string) map[string]*oauthProvider {
	providers := map[string]*oauthProvider{}
	if googleID != "" && googleSecret != "" {
		providers["google"] = &oauthProvider{
			name: "google",
			config: &oauth2.Config{
				ClientID: googleID, ClientSecret: googleSecret,
				Endpoint:    oauth2.Endpoint{AuthURL: ep.googleAuth, TokenURL: ep.googleToken, AuthStyle: oauth2.AuthStyleInParams},
				RedirectURL: webOrigin + "/api/auth/callback/google",
				Scopes:      []string{"openid", "email"},
			},
			usesNonce: true,
			identify:  identifyGoogle([]string{"https://accounts.google.com", "accounts.google.com"}),
		}
	}
	if githubID != "" && githubSecret != "" {
		providers["github"] = &oauthProvider{
			name: "github",
			config: &oauth2.Config{
				ClientID: githubID, ClientSecret: githubSecret,
				Endpoint:    oauth2.Endpoint{AuthURL: ep.githubAuth, TokenURL: ep.githubToken, AuthStyle: oauth2.AuthStyleInParams},
				RedirectURL: webOrigin + "/api/auth/callback/github",
				Scopes:      []string{"user:email"},
			},
			identify: identifyGitHub(ep.githubAPI),
		}
	}
	return providers
}

// oauthStart は POST /api/auth/oauth/{provider}。同意画面の URL を返し、画面がそこへ移る。
// GET で始めないのは、状態（DB の往復の行）を作る処理を別のサイトから踏ませないため（10 D2）。
func (h *authHandlers) oauthStart(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		CallbackURL string `json:"callbackURL"`
	}
	if !readAuthJSON(w, r, &in) {
		return
	}
	p := h.oauth[r.PathValue("provider")]
	if p == nil {
		writeAuthError(w, http.StatusNotFound, "OAUTH_UNAVAILABLE", "このログイン方法は使えません")
		return
	}
	if !h.allowAnonymous(w, r) {
		return
	}
	state, stateHash := newToken()
	verifier := oauth2.GenerateVerifier()
	nonce := base64.RawURLEncoding.EncodeToString(randomBytes(16))
	if err := h.store.saveOAuthState(r.Context(), stateHash, p.name,
		authguard.OAuthState{Verifier: verifier, Nonce: nonce, RedirectTo: safeRedirectPath(in.CallbackURL, "/")}); err != nil {
		internalError(w, r, fmt.Errorf("oauth start: %w", err))
		return
	}
	setCookie(w, oauthCookieName, state, authguard.OAuthStateTTL, http.SameSiteLaxMode)
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if p.usesNonce {
		opts = append(opts, oauth2.SetAuthURLParam("nonce", nonce))
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": p.config.AuthCodeURL(state, opts...)})
}

// oauthCallback は GET /api/auth/callback/{provider}。プロバイダーから画面遷移で戻ってくる。
// 失敗はすべてログインの画面へ戻し、理由の詳しいところはログにだけ残す。
func (h *authHandlers) oauthCallback(w http.ResponseWriter, r *http.Request, previous *session) {
	ctx := r.Context()
	providerName := r.PathValue("provider")
	clearCookie(w, oauthCookieName, http.SameSiteLaxMode)
	fail := func(reason, query string, err error) {
		attrs := []any{"reason", reason, "provider", providerName}
		if err != nil {
			attrs = append(attrs, "err", err.Error())
		}
		logAuthEvent(r, slog.LevelWarn, "oauth_failure", "", attrs...)
		http.Redirect(w, r, "/login?error="+query, http.StatusFound)
	}
	p := h.oauth[providerName]
	q := r.URL.Query()
	if p == nil || q.Get("error") != "" {
		fail("provider_error", "oauth", nil)
		return
	}
	cookie, err := r.Cookie(oauthCookieName)
	if err != nil || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(q.Get("state"))) != 1 {
		fail("state_mismatch", "oauth", nil)
		return
	}
	saved, err := h.store.consumeOAuthState(ctx, q.Get("state"), p.name)
	if err != nil || saved == nil {
		fail("state_unknown", "oauth", err)
		return
	}
	token, err := p.config.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(saved.Verifier))
	if err != nil {
		fail("exchange", "oauth", err)
		return
	}
	ident, err := p.identify(ctx, p.config, token, saved.Nonce, h.clock())
	if err != nil {
		fail("identity", "oauth", err)
		return
	}
	u, err := h.resolveOAuthUser(r, p.name, ident)
	if errors.Is(err, errOAuthEmailNotVerified) {
		fail("email_not_verified", "oauth_email", nil)
		return
	}
	if err != nil {
		fail("resolve_user", "oauth", err)
		return
	}
	if u.Banned {
		logAuthEvent(r, slog.LevelWarn, "sign_in_failure", u.ID, "reason", "banned", "method", p.name)
		http.Redirect(w, r, "/login?error=banned", http.StatusFound)
		return
	}
	// 2段階認証を有効にしている人は、外部ログインのあとにもコードを求める（G3。Better Auth はメール＋パスワードの
	// ログインにしか求めていなかった）。
	if u.MFAEnabled {
		if err := h.startMFAChallenge(w, r, u.ID); err != nil {
			fail("mfa_challenge", "oauth", err)
			return
		}
		http.Redirect(w, r, "/login?mfa=required&callbackURL="+url.QueryEscape(saved.RedirectTo), http.StatusFound)
		return
	}
	if err := h.startSession(w, r, u, false, previous); err != nil {
		fail("session", "oauth", err)
		return
	}
	logAuthEvent(r, slog.LevelInfo, "sign_in_success", u.ID, "method", p.name)
	http.Redirect(w, r, saved.RedirectTo, http.StatusFound)
}

// resolveOAuthUser は外部ログインの利用者を決める（F2）。
//
//  1. プロバイダーと、プロバイダー側の ID の組で結びつきがあれば、その利用者（メールアドレスでは見分けない）
//  2. 無ければ、プロバイダーがメールを確認済みと示していることを求める（示さなければ断る）
//  3. 同じメールアドレスの利用者がいて、その人もメール確認済みなら結びつけ、本人へ知らせる（E5）
//  4. いても未確認なら、その利用者はまだ誰のものとも確かめられていない。相手が先に被害者のメールアドレスで
//     登録しておく乗っ取りを防ぐため、パスワード・セッション・トークンを消してから結びつけ、確認済みにする
//  5. いなければ新しく作り、運営者へ知らせる
func (h *authHandlers) resolveOAuthUser(r *http.Request, provider string, ident *oauthIdentity) (*authUser, error) {
	ctx := r.Context()
	userID, err := h.store.identityUser(ctx, provider, ident.Subject)
	if err != nil {
		return nil, err
	}
	if userID != "" {
		return h.store.findUserByID(ctx, userID)
	}
	email := normalizeEmail(ident.Email)
	if !ident.EmailVerified || !validEmail(email) {
		return nil, errOAuthEmailNotVerified
	}

	now := h.clock()
	userID, event, err := h.store.linkOAuthIdentity(ctx, provider, email, ident, now)
	if err != nil {
		return nil, err
	}

	u, err := h.store.findUserByID(ctx, userID)
	if err != nil || u == nil {
		return nil, fmt.Errorf("find user after oauth: %w (user=%v)", err, u != nil)
	}
	logAuthEvent(r, slog.LevelInfo, "oauth_linked", u.ID, "provider", provider, "how", event)
	switch event {
	case "created":
		h.mailer.notifyAdminOfNewUser(u.Name, u.Email, now)
	case "linked":
		h.mailer.sendAccountLinked(u.Email, provider, h.webOrigin)
	}
	return u, nil
}

// ---- Google（OIDC） ----

// identifyGoogle は ID トークンの中身を確かめる（F1）。ID トークンはトークンエンドポイントから TLS で直接
// 受け取ったものなので、署名の検証は TLS で代える（OIDC Core 3.1.3.7 の 6）。ほかの経路で受け取った
// ID トークンは使わない。
func identifyGoogle(issuers []string) func(context.Context, *oauth2.Config, *oauth2.Token, string, time.Time) (*oauthIdentity, error) {
	return func(_ context.Context, cfg *oauth2.Config, token *oauth2.Token, nonce string, now time.Time) (*oauthIdentity, error) {
		raw, _ := token.Extra("id_token").(string)
		claims, err := parseIDTokenClaims(raw)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(issuers, claims.Issuer) {
			return nil, fmt.Errorf("id_token の iss が違う: %q", claims.Issuer)
		}
		if !claims.Audience.contains(cfg.ClientID) {
			return nil, errors.New("id_token の aud にこのアプリが無い")
		}
		if claims.Expiry == 0 || now.Unix() >= claims.Expiry {
			return nil, errors.New("id_token の期限が切れている")
		}
		if claims.Nonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
			return nil, errors.New("id_token の nonce が違う")
		}
		if claims.Subject == "" {
			return nil, errors.New("id_token に sub が無い")
		}
		return &oauthIdentity{Subject: claims.Subject, Email: claims.Email, EmailVerified: bool(claims.EmailVerified)}, nil
	}
}

type idTokenClaims struct {
	Issuer        string       `json:"iss"`
	Audience      audience     `json:"aud"`
	Expiry        int64        `json:"exp"`
	Nonce         string       `json:"nonce"`
	Subject       string       `json:"sub"`
	Email         string       `json:"email"`
	EmailVerified flexibleBool `json:"email_verified"`
}

// audience は aud（文字列か、文字列の配列）。
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audience) contains(v string) bool { return slices.Contains(a, v) }

// flexibleBool は true と "true" の両方を読む（email_verified を文字列で返すプロバイダーがある）。
type flexibleBool bool

func (f *flexibleBool) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*f = flexibleBool(t)
	case string:
		parsed, _ := strconv.ParseBool(t)
		*f = flexibleBool(parsed)
	}
	return nil
}

func parseIDTokenClaims(raw string) (*idTokenClaims, error) {
	parts := splitJWT(raw)
	if parts == nil {
		return nil, errors.New("id_token が無いか、形が違う")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("id_token の中身が読めない: %w", err)
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("id_token の中身が読めない: %w", err)
	}
	return &claims, nil
}

func splitJWT(raw string) []string {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil
	}
	return parts
}

// ---- GitHub ----

// identifyGitHub は GitHub の API で ID とメールを引く。アクセストークンはここで使ったら捨てる（保存しない。F3）。
// メールは「主で、確認済み」のものを使う。無ければ確認済みのどれか。どれも無ければ未確認として扱う。
func identifyGitHub(apiBase string) func(context.Context, *oauth2.Config, *oauth2.Token, string, time.Time) (*oauthIdentity, error) {
	return func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token, _ string, _ time.Time) (*oauthIdentity, error) {
		client := cfg.Client(ctx, token)
		client.Timeout = externalTimeout
		var user struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := getJSON(ctx, client, apiBase+"/user", &user); err != nil {
			return nil, err
		}
		if user.ID == 0 {
			return nil, errors.New("GitHub の利用者の ID が無い")
		}
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := getJSON(ctx, client, apiBase+"/user/emails", &emails); err != nil {
			return nil, err
		}
		ident := &oauthIdentity{Subject: strconv.FormatInt(user.ID, 10), Name: user.Name, Image: user.AvatarURL}
		if ident.Name == "" {
			ident.Name = user.Login
		}
		for _, e := range emails {
			if e.Verified && (e.Primary || ident.Email == "") {
				ident.Email, ident.EmailVerified = e.Email, true
			}
		}
		return ident, nil
	}
}

func getJSON(ctx context.Context, client *http.Client, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s %d: %s", url, res.StatusCode, detail)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(dst)
}
