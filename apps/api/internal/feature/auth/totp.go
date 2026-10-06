package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// 認証アプリのコード（TOTP、認証基準 10 の G1）と予備コード（G2）。
//
// TOTP は RFC 6238：秘密と「今の時刻を30秒で割った数（時間ステップ）」から HMAC-SHA1 を作り、
// 6桁に縮めたもの。認証アプリ（Google Authenticator など）との互換のため SHA-1・30秒・6桁にする
// （HMAC-SHA1 は HMAC として使う限り、SHA-1 の衝突の弱さは効かない）。
// HMAC は標準の crypto/hmac を使い、ここで書くのは「どのステップを、何個まで、何回まで許すか」だけ。

const (
	totpPeriod     = 30 * time.Second
	totpDigits     = 6
	totpSecretSize = 20 // 160 ビット（G1 の最低値。RFC 4226 の推奨）
	// totpSkew は前後に許すステップの数。端末の時計のずれに1ステップ（30秒）まで付き合う。
	// 広げず、サーバーの時刻は NTP で合わせる（EC2 は Amazon Time Sync Service）。
	totpSkew        = 1
	totpIssuer      = "受験マップ"
	backupCodeCount = 10
	// backupCodeBytes は予備コード1つのランダムさ。10 バイト＝80 ビット。base32 で16文字になる。
	// 80 ビットあれば、DB の SHA-256 から総当たりで戻すことはできない（遅いハッシュは要らない。C2 と同じ考え方）。
	backupCodeBytes = 10
)

// totpStep は時刻の時間ステップ。
func totpStep(t time.Time) int64 {
	return t.Unix() / int64(totpPeriod/time.Second)
}

// totpCode は秘密とステップからコードを作る（RFC 4226 の HOTP。カウンターがステップ）。
func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	// 動的な切り詰め：最後のバイトの下位4ビットが示す位置から4バイトを取り、先頭のビットを落とす。
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000)
}

// matchTOTP は code が Now の前後 totpSkew ステップのどれかに合うかを確かめ、合ったステップを返す。
// lastUsedStep 以下のステップは、合っていても通さない（一度通ったコードを二度と使わせない）。
// 最後の判定（そのステップを使用済みにできたか）は、同時のリクエストに備えて DB の条件つき UPDATE が行う。
func matchTOTP(secret []byte, code string, now time.Time, lastUsedStep *int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	current := totpStep(now)
	matched, found := int64(0), false
	// 合ったかどうかで比べる回数を変えない（どのステップで合ったかが時間に出ないように）。
	for step := current - totpSkew; step <= current+totpSkew; step++ {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 && !found {
			matched, found = step, true
		}
	}
	if !found || (lastUsedStep != nil && matched <= *lastUsedStep) {
		return 0, false
	}
	return matched, true
}

// totpURI は認証アプリに読ませる otpauth の URI（QR コードの中身）。
func totpURI(secret []byte, email string) string {
	label := url.PathEscape(totpIssuer + ":" + email)
	q := url.Values{}
	q.Set("secret", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret))
	q.Set("issuer", totpIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(int(totpPeriod/time.Second)))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// newBackupCodes は予備コードと、DB に置くそのハッシュを返す。表示は作った直後の1回だけ（G2）。
func newBackupCodes() (codes []string, hashes [][]byte) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	for range backupCodeCount {
		raw := strings.ToLower(enc.EncodeToString(randomBytes(backupCodeBytes)))
		// 読み写ししやすいよう4文字ずつ区切って見せる。確かめるときは区切りと大文字・小文字を無視する。
		codes = append(codes, raw[0:4]+"-"+raw[4:8]+"-"+raw[8:12]+"-"+raw[12:16])
		hashes = append(hashes, hashBackupCode(raw))
	}
	return codes, hashes
}

// hashBackupCode は入力された予備コードを DB で引く形にする。区切りと空白、大文字・小文字を無視する。
func hashBackupCode(input string) []byte {
	cleaned := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(input)))
	sum := sha256.Sum256([]byte("backup-code:" + cleaned))
	return sum[:]
}

// totpKeyring は TOTP の秘密を暗号化する鍵の束（G1・I2）。鍵は DB の外（環境変数）に置き、版番号を付ける。
// 暗号化は今の版の鍵、復号は値に書いてある版の鍵で行う。鍵を作り直すときは、新しい版を先頭に足して
// デプロイし（古い版も残す）、各自が次にコードを通したときに新しい鍵で書き直される（totpNeedsReseal）。
type totpKeyring struct {
	current string
	keys    map[string][]byte
}

var errTOTPKeyUnknown = errors.New("TOTP の秘密の鍵の版が見つかりません")

// NewTOTPKeyring は AUTH_TOTP_KEYS（「版:base64 の 32 バイト」をカンマで並べたもの。先頭が今の版）を読む。
//
// 空なら、BETTER_AUTH_SECRET から HKDF で導いた鍵を版 v0 として使う。切り替えの時点で新しい秘密を
// 用意しなくて済むようにするため。BETTER_AUTH_SECRET は Cookie の署名に使わなくなったので、
// この用途だけになる（1つの秘密を複数の用途に使わない。I2）。いずれ AUTH_TOTP_KEYS に版を足して移す。
func NewTOTPKeyring(spec, fallbackSecret string) (*totpKeyring, error) {
	k := &totpKeyring{keys: map[string][]byte{}}
	if fallbackSecret != "" {
		derived, err := hkdf.Key(sha256.New, []byte(fallbackSecret), nil, "juken-map totp secret encryption v0", 32)
		if err != nil {
			return nil, err
		}
		k.keys["v0"] = derived
		k.current = "v0"
	}
	if strings.TrimSpace(spec) == "" {
		if k.current == "" {
			return nil, errors.New("TOTP の秘密を暗号化する鍵がありません（AUTH_TOTP_KEYS か BETTER_AUTH_SECRET）")
		}
		return k, nil
	}
	k.current = ""
	for _, part := range strings.Split(spec, ",") {
		version, encoded, ok := strings.Cut(strings.TrimSpace(part), ":")
		key, err := base64.StdEncoding.DecodeString(encoded)
		if !ok || version == "" || err != nil || len(key) != 32 {
			return nil, fmt.Errorf("AUTH_TOTP_KEYS の %q が「版:base64 の 32 バイト」の形ではありません", version)
		}
		k.keys[version] = key
		if k.current == "" {
			k.current = version
		}
	}
	return k, nil
}

// seal は秘密を AES-256-GCM で暗号化し「版:base64(nonce + 暗号文)」にする。userID を追加データ（AAD）にして、
// 暗号文を別の利用者の行へ写しても復号できないようにする。
func (k *totpKeyring) seal(plain []byte, userID string) (string, error) {
	aead, err := newGCM(k.keys[k.current])
	if err != nil {
		return "", err
	}
	nonce := randomBytes(aead.NonceSize())
	sealed := aead.Seal(nonce, nonce, plain, []byte(userID))
	return k.current + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// open は seal の逆。
func (k *totpKeyring) open(stored, userID string) ([]byte, error) {
	version, encoded, ok := strings.Cut(stored, ":")
	key, known := k.keys[version]
	if !ok || !known {
		return nil, errTOTPKeyUnknown
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize() {
		return nil, errors.New("TOTP の秘密の暗号文が短すぎます")
	}
	return aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(userID))
}

// needsReseal は、今の版でない鍵で暗号化されているか（次にコードを通したときに書き直す）。
func (k *totpKeyring) needsReseal(stored string) bool {
	version, _, _ := strings.Cut(stored, ":")
	return version != k.current
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
