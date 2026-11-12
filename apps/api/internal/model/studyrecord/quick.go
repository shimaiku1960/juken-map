package studyrecord

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// AIクイック記録（JUK-180）の決まり。一文を AI が実績の項目に分けた下書きを、記録の決まりに照らして確かめる。
// 記録はしない（画面がフォームに入れ、利用者が確かめてから CreateLog で記録する）。

// QuickTextMax は一文の長さの上限（コードポイント）。費用と、長い文での指示の書き換えを抑える。
const QuickTextMax = 200

var quickTextRule = validation.StringRule{
	TrimFirst:  true,
	Min:        1,
	MinMessage: "記録したい内容を入力してください",
	Max:        QuickTextMax,
	MaxMessage: fmt.Sprintf("%d文字以内で入力してください", QuickTextMax),
}

// ReadQuickText は本文の text（AI に読ませる一文）を確かめて返す。誤りは *validation.Issue。
func ReadQuickText(attrs any) (string, error) {
	in := validation.ReadObject(attrs)
	text := in.String("text", quickTextRule)
	if err := issue(in); err != nil {
		return "", err
	}
	return text, nil
}

// Draft は AI が一文から読み取った実績の下書き。読み取れなかった項目は nil。
type Draft struct {
	Date       *string
	Subject    *string
	Minutes    *int64
	TextbookID *int64
}

// TextbookRef は AI に見せる参考書（ID・名前・科目。科目が無ければ ""）。
type TextbookRef struct {
	ID      int64
	Name    string
	Subject string
}

// CheckDraft は下書きのうち、記録の決まりに合う項目だけを残す。AI の出力は形こそスキーマどおりでも、
// 中身は信じない。科目は決まった値のどれか、参考書は本人のもの（books）、時間は実績と同じ範囲、
// 日付は暦にある today 以前かを確かめ、外れた項目は nil にする。1つの項目が外れても、ほかの項目は
// 使えるので、全体をエラーにはしない。
func CheckDraft(d Draft, books []TextbookRef, today string) Draft {
	var out Draft
	if v := d.Date; v != nil {
		// time.Parse は暦に無い日（2月30日）をエラーにする。実績の入力（JavaScript と同じく繰り越す）より厳しくてよい。
		if _, err := time.Parse(time.DateOnly, *v); err == nil && *v <= today {
			out.Date = v
		}
	}
	if s := d.Subject; s != nil && IsSubject(*s) {
		out.Subject = s
	}
	if m := d.Minutes; m != nil && *m >= 1 && *m <= MaxMinutes {
		out.Minutes = m
	}
	if id := d.TextbookID; id != nil {
		for _, b := range books {
			if b.ID == *id {
				out.TextbookID = id
				break
			}
		}
	}
	return out
}

// RecentTextbooks は userID の人の参考書を、新しく足したものから limit 冊まで読む（AI に見せる候補）。
func RecentTextbooks(ctx context.Context, db *sql.DB, userID string, limit int) ([]TextbookRef, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, name, COALESCE(subject, '') FROM Textbook WHERE userId = ? ORDER BY id DESC LIMIT ?",
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []TextbookRef
	for rows.Next() {
		var b TextbookRef
		if err := rows.Scan(&b.ID, &b.Name, &b.Subject); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}
