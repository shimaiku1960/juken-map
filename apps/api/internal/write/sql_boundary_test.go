package write_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// apps/api の根（このファイルは apps/api/internal/write にある）。
const apiRoot = "../.."

// 書き込みの SQL を置いてよいディレクトリ（apps/api からの相対）。docs/architecture.md「バックエンドの構成」の決まり5。
// internal/write の外で例外にするのは、最初に決めた2つだけ（マイグレーションの記録とテストデータ）。
// ここに足したくなったら、足す前に決まりそのものを見直す。
var writeAllowed = []string{
	"internal/write/",
	"internal/migrate/", // _prisma_migrations への記録
	"internal/dbtest/",  // テストデータ
}

// writeSQL は書き込みの SQL に見える文字列。
//   - 文の形（INSERT INTO・UPDATE … SET・DELETE FROM・REPLACE INTO）がどこかにある
//   - 文字列が大文字の書き込みの語で始まる（"UPDATE `" + table + "` SET" のように組み立てる SQL の頭）。
//     ルートの "DELETE /api/…" と、画面の操作名の "update"・"delete" は除く
var writeSQL = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(INSERT\s+(IGNORE\s+)?INTO|REPLACE\s+INTO|DELETE\s+FROM)\b`),
	regexp.MustCompile("(?i)\\bUPDATE\\s+`?\\w+`?(\\s+(AS\\s+)?\\w+)?\\s+(SET|JOIN|INNER|LEFT)\\b"),
	regexp.MustCompile(`^\s*(INSERT|UPDATE|DELETE|REPLACE)\s+[^/\s]`),
}

// TestWriteSQLOnlyUnderWrite は、書き込みの SQL が internal/write（と決めた例外）の外に無いことを確かめる。
// _test.go は対象にしない（テストが自分で行を用意するのは構わない）。
func TestWriteSQLOnlyUnderWrite(t *testing.T) {
	inside := 0
	err := filepath.WalkDir(apiRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(apiRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		allowed := false
		for _, dir := range writeAllowed {
			if strings.HasPrefix(rel, dir) {
				allowed = true
			}
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !isWriteSQL(s) {
				return true
			}
			if allowed {
				inside++
				return true
			}
			t.Errorf("%s: 書き込みの SQL が internal/write の外にある。表の持ち主（internal/write/<持ち主>）の操作にする: %.80q",
				fset.Position(lit.Pos()), s)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 探し方が壊れて何も見つけずに通る、ということが無いように。
	if inside < 50 {
		t.Fatalf("internal/write の下で見つけた書き込みの SQL が %d 件しかない。探し方を見直す", inside)
	}
}

func isWriteSQL(s string) bool {
	for _, re := range writeSQL {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func TestIsWriteSQL(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"INSERT INTO `Goal` (userId) VALUES (?)", true},
		{"INSERT IGNORE INTO LineLinkNonce (nonce) VALUES (?)", true},
		{"REPLACE INTO NotificationPreference (userId) VALUES (?)", true},
		{"UPDATE `user` SET name = ? WHERE id = ?", true},
		{"UPDATE StudyPlan p JOIN Textbook t ON t.id = p.textbookId SET p.done = 1", true},
		{"\n\t\tUPDATE StudyLog\n\t\tSET minutes = ?", true},
		{"DELETE FROM AuthSession WHERE expiresAt <= ?", true},
		{"DELETE s FROM AuthSession s JOIN `user` u ON u.id = s.userId", true},
		{"UPDATE `", true}, // "UPDATE `" + table + "` SET …" の頭
		{"SELECT id FROM `user` WHERE email = ?", false},
		{"SELECT id FROM Goal FOR UPDATE", false},
		{"INSERT INTO ... ON DUPLICATE KEY UPDATE", true},
		{"DELETE /api/goals/{id}", false},
		{"POST /api/goals", false},
		{"update", false},
		{"delete", false},
	}
	for _, tt := range tests {
		if got := isWriteSQL(tt.s); got != tt.want {
			t.Errorf("isWriteSQL(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
