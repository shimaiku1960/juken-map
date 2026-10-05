package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// 認証の部品の単体テスト（DB を使わないもの）。DB を使う流れのテストは auth_db_test.go。
// テスト名の B1 などは認証基準 10 の項目の記号。

func TestB1B2PasswordHashFormat(t *testing.T) {
	h := newPasswordHasher(1)
	ctx := context.Background()
	hash, err := h.hash(ctx, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	// アルゴリズムと強さを一緒に保存する（PHC 文字列。B2）。強さは基準の最低値（B1）。
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %s", hash)
	}
	ok, rehash, err := h.verify(ctx, hash, "correct horse battery staple")
	if err != nil || !ok || rehash {
		t.Fatalf("verify = %v %v %v", ok, rehash, err)
	}
	if ok, _, _ := h.verify(ctx, hash, "correct horse battery stapl"); ok {
		t.Fatal("違うパスワードが通った")
	}
	// 同じパスワードでも塩が違うので、毎回違う値になる。
	if again, _ := h.hash(ctx, "correct horse battery staple"); again == hash {
		t.Fatal("塩が使われていない")
	}
}

func TestB2WeakerHashNeedsRehash(t *testing.T) {
	h := newPasswordHasher(1)
	// 弱い設定で作ったハッシュを用意する（一度 hasher の強さを下げて作る）。
	h.params = argon2Params{memoryKiB: 8 * 1024, time: 1, threads: 1}
	weak, _ := h.hash(context.Background(), "an old weak password!")
	h.params = currentArgon2
	ok, rehash, err := h.verify(context.Background(), weak, "an old weak password!")
	if err != nil || !ok || !rehash {
		t.Fatalf("verify = %v %v %v（弱い設定は作り直しが要るはず）", ok, rehash, err)
	}
}

func TestB2BetterAuthScryptIsReadAndRehashed(t *testing.T) {
	// Better Auth（@better-auth/utils の scrypt、N=16384・r=16・p=1）で作った値。Node の同じ計算で作った。
	const legacy = "00112233445566778899aabbccddeeff:c3ed6e7eb77125c0e5bce6a24fb96b9e99e4fdc6e82b2cbdea90bdceb95a421cd78e9bb23c184fdf83e175d3635b65c2673c65efa8eb4e52e4b623b7c36065d8"
	h := newPasswordHasher(1)
	// 全角で作ったパスワードに、半角で入れても合う（どちらも NFKC にそろえる）。
	ok, rehash, err := h.verify(context.Background(), legacy, "legacy-password-1234")
	if err != nil || !ok || !rehash {
		t.Fatalf("verify = %v %v %v", ok, rehash, err)
	}
	if ok, _, _ := h.verify(context.Background(), legacy, "legacy-password-1235"); ok {
		t.Fatal("違うパスワードが通った")
	}
}

func TestB3NormalizesInput(t *testing.T) {
	h := newPasswordHasher(1)
	ctx := context.Background()
	// 全角と半角、合成済みの文字（が：U+304C）と分けた文字（か＋濁点：U+304B U+3099）。
	hash, _ := h.hash(ctx, "ｐａｓｓｗｏｒｄ-が-1234567")
	if ok, _, _ := h.verify(ctx, hash, "password-が-1234567"); !ok {
		t.Fatal("同じ見た目のパスワードで入れない")
	}
}

func TestB3WaitsForSlotOrGivesUp(t *testing.T) {
	h := newPasswordHasher(1)
	h.slots <- struct{}{} // 空きが無い状態にする
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.hash(ctx, "a password that waits"); err == nil {
		t.Fatal("空きを待たずに計算した")
	}
}

func TestA2PasswordRules(t *testing.T) {
	tests := []struct {
		name     string
		password string
		ok       bool
	}{
		{"14文字（パスワードだけでログインできるので足りない）", "kyoto-juken-25", false},
		{"15文字", "kyoto-juken-25x", true},
		{"よく使われているもの（15文字の連番）", "abcdefghijklmno", false},
		{"65文字は受け付ける", strings.Repeat("x", 30) + strings.Repeat("y", 35), true},
		{"256バイトを超える", strings.Repeat("z", 257), false},
		{"よく使われているもの", "passwordpassword", false},
		{"よく使われているもの（大文字）", "PASSWORDPASSWORD", false},
		{"メールアドレスと同じ", "student@example.com", false},
		{"記号や大文字が無くても長ければよい", "i like studying math every day", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := checkNewPassword(tt.password, "Student@example.com")
			if ok != tt.ok {
				t.Errorf("checkNewPassword(%q) = %v, want %v", tt.password, ok, tt.ok)
			}
		})
	}
	if !isCommonPassword("passwordpassword") {
		t.Fatal("一覧が読み込めていない")
	}
}

func TestC2Token(t *testing.T) {
	raw, hash := newToken()
	if len(raw) != 43 || len(hash) != 32 {
		t.Fatalf("raw=%d hash=%d", len(raw), len(hash))
	}
	if got := hashToken(raw); string(got) != string(hash) {
		t.Fatal("同じトークンから同じハッシュにならない")
	}
	for _, bad := range []string{"", "short", strings.Repeat("!", 43), raw + "x"} {
		if hashToken(bad) != nil {
			t.Errorf("形の違う %q を受け付けた", bad)
		}
	}
	other, _ := newToken()
	if other == raw {
		t.Fatal("乱数になっていない")
	}
}

func TestC3Policy(t *testing.T) {
	if p := policyFor("user"); p.absolute != 30*24*time.Hour || p.idle != 0 {
		t.Errorf("user = %+v", p)
	}
	if p := policyFor("admin"); p.absolute != 24*time.Hour || p.idle != time.Hour {
		t.Errorf("admin = %+v", p)
	}
}

func TestG1TOTPMatchesRFC6238(t *testing.T) {
	// RFC 6238 の付録 B（SHA-1）の値の下6桁。
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpCode(secret, totpStep(time.Unix(unix, 0))); got != want {
			t.Errorf("T=%d: %s, want %s", unix, got, want)
		}
	}
}

func TestG1TOTPWindowAndReplay(t *testing.T) {
	secret := randomBytes(totpSecretSize)
	now := time.Unix(1_800_000_000, 0)
	code := func(offset int64) string { return totpCode(secret, totpStep(now)+offset) }
	for _, offset := range []int64{-1, 0, 1} {
		if _, ok := matchTOTP(secret, code(offset), now, nil); !ok {
			t.Errorf("%d ステップずれたコードが通らない", offset)
		}
	}
	for _, offset := range []int64{-3, -2, 2, 3} {
		if _, ok := matchTOTP(secret, code(offset), now, nil); ok {
			t.Errorf("%d ステップずれたコードが通った", offset)
		}
	}
	step, ok := matchTOTP(secret, code(0), now, nil)
	if !ok {
		t.Fatal("通らない")
	}
	// 一度通ったステップ（とそれ以前）は二度と通さない。
	if _, ok := matchTOTP(secret, code(0), now, &step); ok {
		t.Fatal("同じコードが2回通った")
	}
	if _, ok := matchTOTP(secret, code(-1), now, &step); ok {
		t.Fatal("前のステップのコードが通った")
	}
	if _, ok := matchTOTP(secret, code(1), now, &step); !ok {
		t.Fatal("次のステップのコードが通らない")
	}
}

func TestG1KeyringSealAndRotate(t *testing.T) {
	old, err := newTOTPKeyring("", "better-auth-secret")
	if err != nil {
		t.Fatal(err)
	}
	secret := randomBytes(totpSecretSize)
	sealed, err := old.seal(secret, "user-1")
	if err != nil || !strings.HasPrefix(sealed, "v0:") {
		t.Fatalf("sealed = %s, %v", sealed, err)
	}
	if strings.Contains(sealed, base64.StdEncoding.EncodeToString(secret)) {
		t.Fatal("秘密がそのまま入っている")
	}
	// 別の利用者の行へ写した暗号文は復号できない（userID を AAD にしている）。
	if _, err := old.open(sealed, "user-2"); err == nil {
		t.Fatal("別の利用者で復号できた")
	}
	// 新しい版の鍵を足す（古い版は BETTER_AUTH_SECRET から導いたまま残る）。古い値は読めて、書き直しが要ると分かる。
	rotated, err := newTOTPKeyring("v1:"+base64.StdEncoding.EncodeToString(randomBytes(32)), "better-auth-secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := rotated.open(sealed, "user-1")
	if err != nil || string(got) != string(secret) {
		t.Fatalf("古い版の鍵で読めない: %v", err)
	}
	if !rotated.needsReseal(sealed) {
		t.Fatal("古い版なのに書き直しが要らないことになっている")
	}
	resealed, _ := rotated.seal(secret, "user-1")
	if !strings.HasPrefix(resealed, "v1:") || rotated.needsReseal(resealed) {
		t.Fatalf("resealed = %s", resealed)
	}
	if _, err := newTOTPKeyring("v1:short", ""); err == nil {
		t.Fatal("32 バイトでない鍵を受け付けた")
	}
}

func TestG2BackupCodes(t *testing.T) {
	codes, hashes := newBackupCodes()
	if len(codes) != backupCodeCount || len(hashes) != backupCodeCount {
		t.Fatalf("codes=%d hashes=%d", len(codes), len(hashes))
	}
	if len(codes[0]) != 19 {
		t.Fatalf("code = %s", codes[0])
	}
	// 区切りと大文字・小文字を無視して同じハッシュになる。
	if string(hashBackupCode(strings.ToUpper(strings.ReplaceAll(codes[0], "-", " ")))) != string(hashes[0]) {
		t.Fatal("入力の揺れで合わない")
	}
}

func TestF3SafeRedirectPath(t *testing.T) {
	for in, want := range map[string]string{
		"/dashboard":                     "/dashboard",
		"/profile#notification-settings": "/profile#notification-settings",
		"https://evil.example":           "/",
		"//evil.example":                 "/",
		"/\\evil.example":                "/",
		"dashboard":                      "/",
		"":                               "/",
		"/ok\r\nSet-Cookie: x":           "/",
	} {
		if got := safeRedirectPath(in, "/"); got != want {
			t.Errorf("safeRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestF1GoogleIDToken(t *testing.T) {
	cfg := &oauth2.Config{ClientID: "client-1"}
	now := time.Unix(1_800_000_000, 0)
	valid := map[string]any{
		"iss": "https://accounts.google.com", "aud": "client-1", "exp": now.Add(time.Hour).Unix(),
		"nonce": "nonce-1", "sub": "sub-1", "email": "a@example.com", "email_verified": true,
	}
	identify := identifyGoogle([]string{"https://accounts.google.com"})
	run := func(change func(map[string]any)) (*oauthIdentity, error) {
		claims := map[string]any{}
		for k, v := range valid {
			claims[k] = v
		}
		change(claims)
		token := (&oauth2.Token{AccessToken: "x"}).WithExtra(map[string]any{"id_token": fakeIDToken(claims)})
		return identify(context.Background(), cfg, token, "nonce-1", now)
	}
	ident, err := run(func(map[string]any) {})
	if err != nil || ident.Subject != "sub-1" || !ident.EmailVerified {
		t.Fatalf("ident=%+v err=%v", ident, err)
	}
	for name, change := range map[string]func(map[string]any){
		"aud が違う":   func(c map[string]any) { c["aud"] = "other-client" },
		"iss が違う":   func(c map[string]any) { c["iss"] = "https://evil.example" },
		"期限切れ":      func(c map[string]any) { c["exp"] = now.Add(-time.Second).Unix() },
		"nonce が違う": func(c map[string]any) { c["nonce"] = "nonce-2" },
		"nonce が無い": func(c map[string]any) { delete(c, "nonce") },
		"sub が無い":   func(c map[string]any) { delete(c, "sub") },
	} {
		if _, err := run(change); err == nil {
			t.Errorf("%s の ID トークンを受け付けた", name)
		}
	}
	// aud が配列でも、email_verified が文字列でも読める。
	ident, err = run(func(c map[string]any) { c["aud"] = []string{"x", "client-1"}; c["email_verified"] = "true" })
	if err != nil || !ident.EmailVerified {
		t.Fatalf("ident=%+v err=%v", ident, err)
	}
}

// fakeIDToken は署名の無い ID トークン（中身の確かめ方だけを試す。署名は TLS で代えている）。
func fakeIDToken(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

// BenchmarkB1Argon2Hash は、パスワードのハッシュ1回の所要時間とメモリを測る（B1 の測り方）。
// 本番と同じ CPU で測るときは、イメージの中で `go test -run none -bench B1 -benchmem` を流す。
func BenchmarkB1Argon2Hash(b *testing.B) {
	h := newPasswordHasher(1)
	ctx := context.Background()
	for b.Loop() {
		if _, err := h.hash(ctx, "a long passphrase for benchmark"); err != nil {
			b.Fatal(err)
		}
	}
}
