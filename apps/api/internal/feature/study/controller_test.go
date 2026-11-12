package study

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// TestControllerHasNoSQLOrRules は、このパッケージ（コントローラー）が SQL も入力の決まりも持たないことを確かめる。
// 読み書きと入力の決まりはモデル（internal/model/studyrecord）が持つ（docs/architecture.md、JUK-219）。
func TestControllerHasNoSQLOrRules(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			if imp, _ := strconv.Unquote(spec.Path.Value); strings.HasSuffix(imp, "/internal/validation") {
				t.Errorf("%s が %s を import している。入力の決まりはモデルに書く", path, imp)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			head, _, _ := strings.Cut(strings.TrimSpace(strings.ToUpper(s)), " ")
			if slices.Contains([]string{"SELECT", "INSERT", "UPDATE", "DELETE", "WITH"}, head) {
				t.Errorf("%s に SQL がある（%.40q）。読み書きはモデルに書く", path, s)
			}
			return true
		})
	}
}

// TestEnumsMatchContract は、モデルの科目・単位の値が契約（apischema の enum）と同じであることを確かめる。
// モデルは apischema を知らないので、両方を知るコントローラー側で比べる。
func TestEnumsMatchContract(t *testing.T) {
	consts := contractEnums(t)
	for typ, want := range map[string][]string{
		"StudyLogInputSubject":          studyrecord.Subjects,
		"StudyPlanItemInputSubject":     studyrecord.Subjects,
		"StudyPlanUpdateInputSubject":   studyrecord.Subjects,
		"StudyLogParseResultSubject":    studyrecord.Subjects,
		"StudyLogInputRangeUnit":        studyrecord.RangeUnits,
		"StudyPlanItemInputRangeUnit":   studyrecord.RangeUnits,
		"StudyPlanUpdateInputRangeUnit": studyrecord.RangeUnits,
	} {
		got := slices.Sorted(slices.Values(consts[typ]))
		if !slices.Equal(got, slices.Sorted(slices.Values(want))) {
			t.Errorf("%s = %v, モデルは %v", typ, got, want)
		}
	}
}

// contractEnums は apischema の生成コードから、型ごとの enum の値を集める。
func contractEnums(t *testing.T) map[string][]string {
	t.Helper()
	dir := filepath.Join("..", "..", "apischema")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				typ, ok := vs.Type.(*ast.Ident)
				if !ok || len(vs.Values) != 1 {
					continue
				}
				if lit, ok := vs.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					v, _ := strconv.Unquote(lit.Value)
					out[typ.Name] = append(out[typ.Name], v)
				}
			}
		}
	}
	return out
}
