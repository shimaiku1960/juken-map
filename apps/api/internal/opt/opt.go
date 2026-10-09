// Package opt は、持ち主（internal/write）の操作に渡す「送られなかった」と「null」を区別する値。
// 入口が本文を読んだ結果（httpx.Optional）を、持ち主が import できる形にしたもの。
// 持ち主ではないので internal/write の外に置く。入口の土台（httpx）が write に依存しないため（JUK-160）。
package opt

// Field は Present が false ならキーが無く、Value が nil なら null。
type Field[T comparable] struct {
	Present bool
	Value   *T
}

// Of は値が送られたことを表す。
func Of[T comparable](v T) Field[T] { return Field[T]{Present: true, Value: &v} }

// Differs は送られた値が current（DB の値で、null か値）と違うか。
// キーが無いときも true を返す（呼び出し側は、確かめ直す側に倒す）。
func (f Field[T]) Differs(current *T) bool {
	switch {
	case !f.Present:
		return true
	case f.Value == nil || current == nil:
		return f.Value != current
	}
	return *f.Value != *current
}

// Or は、送られていれば送られた値（null を含む）を、送られていなければ fallback を返す。
func (f Field[T]) Or(fallback *T) *T {
	if f.Present {
		return f.Value
	}
	return fallback
}
