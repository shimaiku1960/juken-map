package write_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeDenied は、書き込みの持ち主が import してはいけないパッケージ（この頭で始まるもの全部）。
// docs/architecture.md「バックエンドの構成」の決まり2（write は HTTP を知らない）。
var writeDenied = []string{
	"net/http",
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx",     // httpxtest も含む
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema", // 画面に返す形
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/",
	"github.com/shimaiku1960/juken-map/apps/api/internal/app",
	"github.com/shimaiku1960/juken-map/apps/api/internal/spa",
}

// TestWriteDoesNotKnowHTTP は、internal/write の下が HTTP と画面の形に依存していないことを確かめる（JUK-159）。
// _test.go も対象にする。
func TestWriteDoesNotKnowHTTP(t *testing.T) {
	files := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files++
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
			for _, denied := range writeDenied {
				if imp == denied || strings.HasPrefix(imp, strings.TrimSuffix(denied, "/")+"/") {
					t.Errorf("%s: internal/write が %s を import している。write は HTTP を知らない"+
						"（docs/architecture.md「バックエンドの構成」の決まり2）。操作の引数と結果の型は持ち主が自分で決め、"+
						"画面の形への変換は feature で行う", fset.Position(spec.Pos()), imp)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 探し方が壊れて何も見ずに通る、ということが無いように。
	if files < 20 {
		t.Fatalf("internal/write の下で見た Go のファイルが %d 個しかない。探し方を見直す", files)
	}
}
