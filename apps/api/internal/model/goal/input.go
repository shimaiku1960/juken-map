package goal

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり（Rails の validates にあたる）。
//
// 操作は送られた本文（JSON を読んだ値。数は json.Number）をそのまま受け取り、ここで形と値を確かめてから書く。
// 確かめ済みの値の型は外に出さないので、入力チェックを通らずに書き込むことはできない。
// 誤りは最初の1件を *validation.Issue で返し、コントローラーがそれを 400 にする。
//
// 規則の正は Zod の goalSchema・updateGoalSchema・patchGoalSchema（src/shared/validations/goal.ts）。

// Statuses は GOAL_STATUSES（Zod のスキーマに書いた順。z.enum の文言に出る）。
var Statuses = []string{"candidate", "decided"}

var (
	// facultyIDRule は z.number().int().positive("志望学部を選択してください")。
	facultyIDRule = validation.NumberRule{Int: true, Positive: true, PositiveMessage: "志望学部を選択してください"}
	// noteRule は z.string().max(500, …)（削らない。null は .nullable() で別に受ける）。
	noteRule = validation.StringRule{Max: 500, MaxMessage: "500文字以内で入力してください"}
)

// createInput は登録の本文（goalSchema）。
type createInput struct {
	facultyID int64
	status    *string
}

func decodeCreate(body any) (createInput, error) {
	in := validation.ReadObject(body)
	facultyID := int64(in.Number("facultyId", facultyIDRule))
	status := in.OptionalEnum("status", Statuses, false)
	return createInput{facultyID: facultyID, status: status.Ptr()}, in.Err()
}

// decodeReplace は差し替えの本文（updateGoalSchema。goalSchema の全項目を任意にしたもの）。
// status も確かめるが、書くのは学部だけなので学部だけを返す。
func decodeReplace(body any) (*int64, error) {
	in := validation.ReadObject(body)
	facultyID := in.OptionalInt("facultyId", facultyIDRule, false)
	in.OptionalEnum("status", Statuses, false)
	return facultyID.Ptr(), in.Err()
}

// patch は第一志望・メモ・ステータスの変更（patchGoalSchema）。Present の項目だけを書き換える。
type patch struct {
	isFirstChoice opt.Field[bool]
	note, status  opt.Field[string]
}

func decodePatch(body any) (patch, error) {
	in := validation.ReadObject(body)
	p := patch{
		isFirstChoice: in.OptionalBool("isFirstChoice").Field(),
		note:          in.OptionalString("note", noteRule, true).Field(),
		status:        in.OptionalEnum("status", Statuses, false).Field(),
	}
	return p, in.Err()
}
