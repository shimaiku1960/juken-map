package feature_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// featurePrefix は feature のパッケージの import パスの頭。
const featurePrefix = "github.com/shimaiku1960/juken-map/apps/api/internal/feature/"

// TestFeaturesDoNotImportEachOther は、feature が別の feature を import していないことを確かめる。
// docs/architecture.md「バックエンドの構成」の決まり1（feature 同士は呼び合わない、JUK-159）。
// _test.go も対象にする（テストの補助を借りるのも、feature 同士をつなぐことになる）。
func TestFeaturesDoNotImportEachOther(t *testing.T) {
	features := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// path は "study/logs.go" の形。最初の段が自分の feature。
		own, _, ok := strings.Cut(filepath.ToSlash(path), "/")
		if !ok {
			return nil // internal/feature 直下のこのファイル
		}
		features[own] = true
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			rest, found := strings.CutPrefix(imp, featurePrefix)
			if !found {
				continue
			}
			other, _, _ := strings.Cut(rest, "/")
			if other != own {
				t.Errorf("%s: feature/%s が feature/%s を import している。feature 同士は呼び合わない"+
					"（docs/architecture.md「バックエンドの構成」の決まり1）。読み取りは自分で書き、変更は internal/write の操作を呼ぶ。"+
					"同じ読み取りを使う入口なら、1つの feature にまとめる（ダッシュボードを study に含めたように）",
					fset.Position(spec.Pos()), own, other)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 探し方が壊れて何も見ずに通る、ということが無いように。
	if len(features) < 10 {
		t.Fatalf("見つけた feature が %d 個しかない。探し方を見直す", len(features))
	}
}
