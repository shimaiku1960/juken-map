// Package opt は、持ち主（internal/write）の操作に渡す「送られなかった」と「null」を区別する値。
// 入口が本文を読んだ結果（package main の optional）を、持ち主が import できる形にしたもの。
package opt

// Field は Present が false ならキーが無く、Value が nil なら null。
type Field[T comparable] struct {
	Present bool
	Value   *T
}

// Of は値が送られたことを表す。
func Of[T comparable](v T) Field[T] { return Field[T]{Present: true, Value: &v} }

// Differs は Node の `data.x !== current`（current は DB の値で、null か値）。
// キーが無い（undefined）ときは、どんな値とも違う（undefined !== null も true）。
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
