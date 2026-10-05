package main

import (
	"crypto/sha256"
	"encoding/base64"
)

// 乱数のトークン（セッション・メールのリンク・2段階認証の途中・OAuth の state）の作り方（認証基準 10 の C2・E1）。
//
// 256 ビットの乱数（CSPRNG）を URL に載せられる base64 にして渡し、DB には SHA-256 だけを置く。
// 受け取ったトークンは SHA-256 にしてから引く。DB が漏れても、そこにある値からトークンは作れない。
// 塩や遅いハッシュは要らない。元が 256 ビットの乱数なので、ハッシュから総当たりで戻すことができない
// （パスワードのように人が選んだ、候補を絞れる値とは違う）。
//
// Cookie に署名は付けない。盗まれた Cookie は署名ごと使えるので守りにならず、DB が漏れたときの守りは
// ハッシュで保存することが担う（Better Auth は署名を付けていた）。

const tokenBytes = 32

// newToken は新しいトークンと、DB に置くそのハッシュを返す。
func newToken() (raw string, hash []byte) {
	raw = base64.RawURLEncoding.EncodeToString(randomBytes(tokenBytes))
	return raw, hashToken(raw)
}

// hashToken は受け取ったトークンを DB で引く形にする。形が違う値（長さ・文字）は、DB を引かずに
// 済むよう nil を返す（ありもしないトークンで DB を叩かせない）。
func hashToken(raw string) []byte {
	if len(raw) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return nil
	}
	if _, err := base64.RawURLEncoding.DecodeString(raw); err != nil {
		return nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
