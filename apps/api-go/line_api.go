package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// LINE の API を呼ぶ部分（JUK-79）。Node の infra/line.ts（Messaging API）と infra/lineLogin.ts（LINE Login）にあたる。
// DB も HTTP の入口も知らない。ハンドラ（line.go）はこの interface を通して呼ぶので、テストでは偽物を渡せる。

type lineClient interface {
	// Messaging API（公式アカウントのトーク）
	issueLinkToken(ctx context.Context, lineUserID string) (string, error)
	replyText(ctx context.Context, replyToken, text string) error
	pushText(ctx context.Context, lineUserID, text string) error

	// LINE Login（利用者の LINE アカウントを確かめる OAuth）
	authCodeURL(state, nonce, codeVerifier, redirectURI string) (string, error)
	exchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (lineTokens, error)
	verifyIDToken(ctx context.Context, idToken, nonce string) (lineIdentity, error)
	isFriend(ctx context.Context, accessToken string) (bool, error)
}

type lineTokens struct {
	AccessToken string
	IDToken     string
}

// lineIdentity は ID トークンを LINE に確かめてもらった結果。Sub が LINE のユーザー ID。
type lineIdentity struct {
	Sub   string `json:"sub"`
	Nonce string `json:"nonce"`
}

// verifyLineSignature は Webhook の署名（x-line-signature）を確かめる。
// 署名はチャネルシークレットで本文を HMAC-SHA256 したものの base64。本文は JSON として読む前の、
// 受け取ったままのバイト列で計算する（読んでから書き直すと、空白やキーの順番が変わって一致しない）。
// hmac.Equal は時間が一定の比べ方（== だと何バイト目まで合っていたかが応答時間に出る）。
// シークレットが空なら必ず false（設定し忘れで誰の送ったものでも通る状態にしない）。
func verifyLineSignature(body []byte, signature string, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// lineAccountLinkURL は、LINE のトークで案内する「連携する」リンク。画面の /line/link が受ける。
func lineAccountLinkURL(linkToken string) string {
	return siteURL + "/line/link?" + url.Values{"linkToken": {linkToken}}.Encode()
}

// httpLineClient は LINE の API を直接呼ぶ（SDK は使わない。Node も同じ）。
type httpLineClient struct {
	client *http.Client
	// botBase は Messaging API の根元（既定 https://api.line.me/v2/bot）。
	// 毎日の通知と同じ LINE_API_BASE で、手元の比較やテストでは偽のサーバーへ向ける。
	botBase     string
	accessToken string // LINE_CHANNEL_ACCESS_TOKEN
	// loginBase は LINE Login の API の根元（既定 https://api.line.me）。authorizeURL は利用者のブラウザが開く同意画面。
	loginBase      string
	authorizeURL   string
	loginChannelID string // LINE_LOGIN_CHANNEL_ID
	loginSecret    string // LINE_LOGIN_CHANNEL_SECRET
}

func (c *httpLineClient) issueLinkToken(ctx context.Context, lineUserID string) (string, error) {
	var res struct {
		LinkToken string `json:"linkToken"`
	}
	err := c.botRequest(ctx, "/user/"+url.PathEscape(lineUserID)+"/linkToken", nil, &res)
	return res.LinkToken, err
}

func (c *httpLineClient) replyText(ctx context.Context, replyToken, text string) error {
	return c.botRequest(ctx, "/message/reply", map[string]any{
		"replyToken": replyToken, "messages": []map[string]string{{"type": "text", "text": text}},
	}, nil)
}

func (c *httpLineClient) pushText(ctx context.Context, lineUserID, text string) error {
	return c.botRequest(ctx, "/message/push", map[string]any{
		"to": lineUserID, "messages": []map[string]string{{"type": "text", "text": text}},
	}, nil)
}

// botRequest は Messaging API へ POST する。body が nil なら本文なし、out が nil なら応答を読み捨てる。
func (c *httpLineClient) botRequest(ctx context.Context, path string, body, out any) error {
	if c.accessToken == "" {
		return errors.New("LINE_CHANNEL_ACCESS_TOKEN is not configured")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(raw))
	}
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.botBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

// oauthConfig は LINE Login の OAuth 2.0 の設定。redirectURI は開始のときに DB へ残した値を使う
// （開始と戻りで同じ値でないと、LINE がトークンの交換を断る）。
func (c *httpLineClient) oauthConfig(redirectURI string) (*oauth2.Config, error) {
	if c.loginChannelID == "" {
		return nil, errors.New("LINE_LOGIN_CHANNEL_ID is not configured")
	}
	if c.loginSecret == "" {
		return nil, errors.New("LINE_LOGIN_CHANNEL_SECRET is not configured")
	}
	return &oauth2.Config{
		ClientID:     c.loginChannelID,
		ClientSecret: c.loginSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.authorizeURL,
			TokenURL: c.loginBase + "/oauth2/v2.1/token",
			// LINE は client_id と client_secret を本文で受け取る（Basic 認証のヘッダーではない）。
			// 決めておかないと、oauth2 は両方の形を試すために1回余計に呼ぶ。
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURI,
		Scopes:      []string{"openid", "profile"},
	}, nil
}

// authCodeURL は LINE の同意画面の URL。PKCE（S256）と nonce を付ける。
// bot_prompt=aggressive は、同意画面で公式アカウントの友だち追加も勧める指定（友だちでないと通知を送れない）。
func (c *httpLineClient) authCodeURL(state, nonce, codeVerifier, redirectURI string) (string, error) {
	cfg, err := c.oauthConfig(redirectURI)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("bot_prompt", "aggressive"),
		oauth2.S256ChallengeOption(codeVerifier),
	), nil
}

func (c *httpLineClient) exchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (lineTokens, error) {
	cfg, err := c.oauthConfig(redirectURI)
	if err != nil {
		return lineTokens{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	// oauth2 は ctx に入れた *http.Client で呼ぶ（入れなければ http.DefaultClient）。
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.client)
	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return lineTokens{}, fmt.Errorf("LINE Login token: %w", err)
	}
	// ID トークンは OAuth 2.0 の決まった項目ではないので、Extra から取り出す。
	idToken, _ := token.Extra("id_token").(string)
	if idToken == "" {
		return lineTokens{}, errors.New("LINE Login token: id_token がありません")
	}
	return lineTokens{AccessToken: token.AccessToken, IDToken: idToken}, nil
}

// verifyIDToken は ID トークンの署名・期限・宛先（client_id）・nonce を LINE に確かめてもらう。
// 自分で JWT を検証せず LINE の verify を呼ぶのは Node と同じ（鍵の取得と更新を持たずに済む）。
func (c *httpLineClient) verifyIDToken(ctx context.Context, idToken, nonce string) (lineIdentity, error) {
	var identity lineIdentity
	if c.loginChannelID == "" {
		return identity, errors.New("LINE_LOGIN_CHANNEL_ID is not configured")
	}
	form := url.Values{"id_token": {idToken}, "client_id": {c.loginChannelID}, "nonce": {nonce}}
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.loginBase+"/oauth2/v2.1/verify", strings.NewReader(form.Encode()))
	if err != nil {
		return identity, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return identity, c.do(req, &identity)
}

// isFriend は、その利用者が公式アカウントを友だちにしているか（ブロック中も false）。
func (c *httpLineClient) isFriend(ctx context.Context, accessToken string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.loginBase+"/friendship/v1/status", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	var res struct {
		FriendFlag bool `json:"friendFlag"`
	}
	err = c.do(req, &res)
	return res.FriendFlag, err
}

// do は送って、2xx でなければ本文の先頭をエラーに入れる。out が nil なら応答を読み捨てる。
func (c *httpLineClient) do(req *http.Request, out any) error {
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		// URL の path には LINE のユーザー ID が入ることがあるので、エラー（ログに出る）には出さない。
		return fmt.Errorf("LINE API %d: %s", res.StatusCode, detail)
	}
	if out == nil {
		// 本文を読み切ると、接続を使い回せる。
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
