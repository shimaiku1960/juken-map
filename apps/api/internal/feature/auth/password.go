package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
)

// パスワードの保存（認証基準 10 の B1〜B3）と、新しく決めるパスワードの規則（A2）。
//
// 暗号の部品（Argon2id・scrypt・乱数）は x/crypto と標準ライブラリのものを使い、自分では書かない。
// ここで書くのは「どの強さで計算し、どの形で保存し、どう比べるか」の流れだけ。

// argon2Params は Argon2id の強さ。保存する文字列（PHC 文字列）に一緒に書くので、後から強さを
// 変えても、古い値は書いてある強さで確かめられる（B2）。
type argon2Params struct {
	memoryKiB uint32
	time      uint32
	threads   uint8
}

// currentArgon2 は新しく作るハッシュの強さ。基準 B1 の Argon2id の最低値（m=19MiB・t=2・p=1）。
//
// 本番の EC2（t3.micro、メモリ 908MB）は平常時でも Swap を使っているので、最低値から始める。
// 1回の計算に 19MiB を使い、同時に計算する数（hashConcurrency）で上限を決める。強くするのは、
// 本番で1回の所要時間を測って、サインインの p95 < 800ms（02 B3）に余裕があると分かってから。
var currentArgon2 = argon2Params{memoryKiB: 19 * 1024, time: 2, threads: 1}

const (
	argon2KeyLen  = 32
	argon2SaltLen = 16
	// DefaultHashConcurrency は同時にハッシュを計算する数の既定値。計算は CPU を使い切るので、
	// vCPU の数（本番は2）より多く並べても速くならず、メモリ（1つ 19MiB）が増えるだけ。
	// 待っているリクエストは、リクエストの上限時間（main.go の requestTimeout）で打ち切られる。
	DefaultHashConcurrency = 2
	// passwordMinRunes はパスワードの最低の長さ（A2）。パスワードだけでログインできるので 15 文字。
	// 2段階認証を必須にしているのは管理者だけで、一般の利用者はパスワードだけで入れる。
	passwordMinRunes = 15
	// passwordMaxBytes はハッシュの前にかける上限（A2）。64 文字以上は必ず受け付け、
	// 極端に長い入力でハッシュの計算を重くされないよう 256 バイトで切る。
	passwordMaxBytes = 256
)

// passwordHasher はパスワードのハッシュを作り、確かめる。同時に計算する数を slots で絞る。
type passwordHasher struct {
	params argon2Params
	slots  chan struct{}
	// dummy は「存在しない利用者」のときに確かめるハッシュ（B3）。存在するときと同じだけ計算してから断り、
	// 応答の時間の差で登録の有無が分からないようにする。中身は誰も知らない乱数のパスワード。
	dummy string
}

func newPasswordHasher(concurrency int) *passwordHasher {
	if concurrency < 1 {
		concurrency = DefaultHashConcurrency
	}
	h := &passwordHasher{params: currentArgon2, slots: make(chan struct{}, concurrency)}
	h.dummy = encodeArgon2(currentArgon2, randomBytes(argon2SaltLen), randomBytes(argon2KeyLen))
	return h
}

var errPasswordFormat = errors.New("パスワードのハッシュの形が読めません")

// hash は新しいパスワードのハッシュを PHC 文字列で返す。
func (h *passwordHasher) hash(ctx context.Context, password string) (string, error) {
	salt := randomBytes(argon2SaltLen)
	var key []byte
	err := h.withSlot(ctx, func() {
		key = argon2.IDKey([]byte(normalizePassword(password)), salt, h.params.time, h.params.memoryKiB, h.params.threads, argon2KeyLen)
	})
	if err != nil {
		return "", err
	}
	return encodeArgon2(h.params, salt, key), nil
}

// verify は password が encoded（保存してあるハッシュ）に合うかを返す。needsRehash は、合っていて、
// かつ保存してある方式が今の基準より弱いとき true（呼び出し側がその場で作り直して保存する。B2）。
func (h *passwordHasher) verify(ctx context.Context, encoded, password string) (ok, needsRehash bool, err error) {
	if len(password) > passwordMaxBytes {
		return false, false, nil
	}
	switch {
	case strings.HasPrefix(encoded, "$argon2id$"):
		params, salt, want, err := decodeArgon2(encoded)
		if err != nil {
			return false, false, err
		}
		var got []byte
		err = h.withSlot(ctx, func() {
			got = argon2.IDKey([]byte(normalizePassword(password)), salt, params.time, params.memoryKiB, params.threads, uint32(len(want)))
		})
		if err != nil {
			return false, false, err
		}
		// == で比べると、先頭から何バイト合っているかが時間に出る。一定時間で比べる（B3）。
		if subtle.ConstantTimeCompare(got, want) != 1 {
			return false, false, nil
		}
		return true, params.weakerThan(h.params), nil
	case isBetterAuthScrypt(encoded):
		ok, err := h.verifyBetterAuthScrypt(ctx, encoded, password)
		// 合っていれば必ず Argon2id に作り直す。
		return ok, ok, err
	default:
		return false, false, errPasswordFormat
	}
}

// verifyDummy は存在しない利用者のときに呼ぶ。結果は捨てる（B3）。
func (h *passwordHasher) verifyDummy(ctx context.Context, password string) {
	_, _, _ = h.verify(ctx, h.dummy, password)
}

// withSlot は空きを待ってから f を動かす。待っているあいだにリクエストが打ち切られたら、計算せずに返す。
func (h *passwordHasher) withSlot(ctx context.Context, f func()) error {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-h.slots }()
	f()
	return nil
}

// normalizePassword は、見た目が同じで符号が違う文字（全角・半角、合成済みの文字）を同じにする（B3）。
// 方式は NFKC に固定する。変えると、今までのパスワードでログインできなくなる（Better Auth も NFKC だった）。
func normalizePassword(password string) string {
	return norm.NFKC.String(password)
}

func (p argon2Params) weakerThan(current argon2Params) bool {
	return p.memoryKiB < current.memoryKiB || p.time < current.time
}

// encodeArgon2 は PHC 文字列の形（$argon2id$v=19$m=19456,t=2,p=1$塩$ハッシュ）にする。
// 塩とハッシュは = の無い base64（PHC の決まり）。区切りの形を自分で作らない（B2）。
func encodeArgon2(p argon2Params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.memoryKiB, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func decodeArgon2(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", 塩, ハッシュ
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errPasswordFormat
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, errPasswordFormat
	}
	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memoryKiB, &p.time, &p.threads); err != nil {
		return argon2Params{}, nil, nil, errPasswordFormat
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, errPasswordFormat
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return argon2Params{}, nil, nil, errPasswordFormat
	}
	return p, salt, key, nil
}

// Better Auth の scrypt（@better-auth/utils の password.node.mjs）。保存の形は「塩の hex:ハッシュの hex」で、
// 強さは書かれていない（N=16384・r=16・p=1・64 バイトで固定）。塩は hex の文字列のまま scrypt に渡している。
// この強さは基準 B1 の scrypt の最低値（N=2^17・r=8）に届かないので、ログインに成功したら Argon2id に作り直す。
const (
	betterAuthScryptN      = 16384
	betterAuthScryptR      = 16
	betterAuthScryptP      = 1
	betterAuthScryptKeyLen = 64
)

func isBetterAuthScrypt(encoded string) bool {
	salt, key, ok := strings.Cut(encoded, ":")
	return ok && len(salt) == 32 && len(key) == betterAuthScryptKeyLen*2
}

func (h *passwordHasher) verifyBetterAuthScrypt(ctx context.Context, encoded, password string) (bool, error) {
	salt, keyHex, _ := strings.Cut(encoded, ":")
	want, err := hex.DecodeString(keyHex)
	if err != nil {
		return false, errPasswordFormat
	}
	var got []byte
	var keyErr error
	err = h.withSlot(ctx, func() {
		got, keyErr = scrypt.Key([]byte(normalizePassword(password)), []byte(salt),
			betterAuthScryptN, betterAuthScryptR, betterAuthScryptP, betterAuthScryptKeyLen)
	})
	if err != nil {
		return false, err
	}
	if keyErr != nil {
		return false, keyErr
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// checkNewPassword は、新しく決めるパスワードが規則（A2）に合うかを確かめ、合わなければ画面に出す文言を返す。
// 文字の種類の組み合わせは求めない（Password1! のような予測できる形を増やすだけなので）。定期的な変更も求めない。
func checkNewPassword(password, email string) (message string, ok bool) {
	if len(password) > passwordMaxBytes {
		return "パスワードが長すぎます（半角で256文字まで）", false
	}
	normalized := normalizePassword(password)
	if utf8.RuneCountInString(normalized) < passwordMinRunes {
		return fmt.Sprintf("パスワードは%d文字以上にしてください", passwordMinRunes), false
	}
	lower := strings.ToLower(normalized)
	if email != "" && lower == strings.ToLower(strings.TrimSpace(email)) {
		return "メールアドレスと同じパスワードは使えません", false
	}
	if isCommonPassword(lower) {
		return "よく使われている（または漏えいしたことのある）パスワードのため使えません。別のパスワードにしてください", false
	}
	return "", true
}

//go:embed common_passwords.txt
var commonPasswordsText string

// commonPasswords は一覧を初めて使うときに1回だけ読み込む（使わない起動ではメモリを取らない）。
var commonPasswords = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{}, 16384)
	sc := bufio.NewScanner(strings.NewReader(commonPasswordsText))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = struct{}{}
	}
	return set
})

func isCommonPassword(lower string) bool {
	_, found := commonPasswords()[lower]
	return found
}

// randomBytes は暗号用の乱数（CSPRNG）。Go 1.24 からの crypto/rand.Read は失敗しない（失敗すると落ちる）。
func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
