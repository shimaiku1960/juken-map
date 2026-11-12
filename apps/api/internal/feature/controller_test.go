package feature_test

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

	"github.com/shimaiku1960/juken-map/apps/api/internal/model/goal"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbook"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbookmaster"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// controllers は、モデルに移した機能のコントローラー（internal/feature の下のパッケージ）。
// 機能をモデルに移したら、ここに足す（docs/architecture.md「モデル・コントローラー・ビュー」、JUK-219・JUK-221）。
var controllers = []string{"chaos", "goals", "line", "notifications", "study", "textbooks", "universities"}

// TestControllerHasNoSQLOrRules は、コントローラーが SQL も入力の決まりも持たないことを確かめる。
// 読み書きと入力の決まりはモデル（internal/model の下）が持つ。
func TestControllerHasNoSQLOrRules(t *testing.T) {
	for _, dir := range controllers {
		t.Run(dir, func(t *testing.T) { checkController(t, dir) })
	}
}

func checkController(t *testing.T, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("%s に Go のファイルが無い。controllers の名前を見直す", dir)
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
			// 「delete」のような操作の名前の1語は SQL とみなさない（キーワードのあとに続きがあるものだけ）。
			words := strings.Fields(strings.ToUpper(s))
			if len(words) >= 2 && slices.Contains([]string{"SELECT", "INSERT", "UPDATE", "DELETE", "WITH"}, words[0]) {
				t.Errorf("%s に SQL がある（%.40q）。読み書きはモデルに書く", path, s)
			}
			return true
		})
	}
}

// TestEnumsMatchContract は、モデルの選べる値（科目・単位・志望校のステータスなど）が契約（apischema の enum）と
// 同じであることを確かめる。モデルは apischema を知らないので、両方を知るここで比べる。
func TestEnumsMatchContract(t *testing.T) {
	consts := contractEnums(t)
	for typ, want := range map[string][]string{
		"StudyLogInputSubject":           studyrecord.Subjects,
		"StudyPlanItemInputSubject":      studyrecord.Subjects,
		"StudyPlanUpdateInputSubject":    studyrecord.Subjects,
		"StudyLogParseResultSubject":     studyrecord.Subjects,
		"StudyLogInputRangeUnit":         studyrecord.RangeUnits,
		"StudyPlanItemInputRangeUnit":    studyrecord.RangeUnits,
		"StudyPlanUpdateInputRangeUnit":  studyrecord.RangeUnits,
		"GoalInputStatus":                goal.Statuses,
		"GoalUpdateInputStatus":          goal.Statuses,
		"GoalPatchInputStatus":           goal.Statuses,
		"TextbookNameInputSubject":       textbook.Subjects,
		"TextbookProgressInputSubject":   textbook.Subjects,
		"TextbookNameInputRangeUnit":     textbook.RangeUnits,
		"TextbookProgressInputRangeUnit": textbook.RangeUnits,
		"TextbookMasterInputMetricsUnit": textbookmaster.Units,
		"UniversityInputType":            university.Types,
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
	dir := filepath.Join("..", "apischema")
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
