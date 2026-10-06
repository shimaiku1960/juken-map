package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 参考書の読み取り（JUK-73）。Node の routes/textbooks.ts・textbook-masters.ts の GET と、
// services/textbook-service.ts の listTextbooks・listTextbookMasters にあたる。
// 書き込み（POST /api/textbooks・PATCH /api/textbooks/:id）は textbook_writes.go。

// ここから下の型が応答の形。Node は DB の行をそのまま返している（src/shared/dto には無い）ので、
// 列の名前と並びも Node の TEXTBOOK_COLUMNS・groupMasters に揃える。
// 日時は Date を JSON にしたときと同じ ISO 文字列。

type textbookStore struct {
	db *sql.DB
	// masters は GET /api/textbook-masters の JSON。全員に同じで、変わるのは管理画面の編集
	// （admin_textbook_masters.go）だけなので、メモリに持つ（internal/httpx/json_snapshot.go、JUK-50）。
	masters *httpx.JSONSnapshotCache
}

// textbookMastersCacheTTL は、DB を直接書き換えたとき（seed など）の保険。大学一覧と同じ10分。
const textbookMastersCacheTTL = 10 * time.Minute

func newTextbookStore(db *sql.DB) *textbookStore {
	st := &textbookStore{db: db}
	st.masters = httpx.NewJSONSnapshotCache(textbookMastersCacheTTL, func(ctx context.Context) (any, error) {
		return st.listTextbookMasters(ctx)
	})
	return st
}

// textbookRowColumns は Node の TEXTBOOK_COLUMNS と同じ列と並び。scanTextbook で読む。
const textbookRowColumns = "id, userId, masterId, name, totalAmount, rangeUnit, targetDate, subject, createdAt, updatedAt"

// scanTextbook は textbookRowColumns の1行を読み、日時を ISO にする。scan は rows.Scan か row.Scan。
func scanTextbook(scan func(...any) error) (apischema.TextbookRow, error) {
	var t apischema.TextbookRow
	if err := scan(
		&t.ID, &t.UserID, &t.MasterID, &t.Name, &t.TotalAmount, &t.RangeUnit,
		&t.TargetDate, &t.Subject, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return t, err
	}
	if t.TargetDate != nil {
		iso := database.ISOFromDatetime(*t.TargetDate)
		t.TargetDate = &iso
	}
	t.CreatedAt = database.ISOFromDatetime(t.CreatedAt)
	t.UpdatedAt = database.ISOFromDatetime(t.UpdatedAt)
	return t, nil
}

// listTextbooks は自分の参考書の一覧。名前は (userId, name) で UNIQUE なので、名前順だけで並びが決まる。
func (st *textbookStore) listTextbooks(ctx context.Context, userID string) ([]apischema.TextbookRow, error) {
	rows, err := st.db.QueryContext(ctx,
		"SELECT "+textbookRowColumns+" FROM Textbook WHERE userId = ? ORDER BY name ASC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	textbooks := make([]apischema.TextbookRow, 0)
	for rows.Next() {
		t, err := scanTextbook(rows.Scan)
		if err != nil {
			return nil, err
		}
		textbooks = append(textbooks, t)
	}
	return textbooks, rows.Err()
}

// listTextbookMasters は参考書マスターの一覧を、総量の候補（metrics）と一緒に返す。全員に同じもの。
// マスター → 総量の候補は1対多なので、LEFT JOIN 1本で取り、マスターごとに束ねる。
// 候補は id 順（登録時に「isDefault の候補、無ければ先頭」を使うので、先頭を決めておく）。
func (st *textbookStore) listTextbookMasters(ctx context.Context) ([]apischema.TextbookMaster, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn, tm.createdAt, tm.updatedAt,
		        m.id AS m_id, m.unit AS m_unit, m.totalAmount AS m_totalAmount,
		        m.isDefault AS m_isDefault, m.createdAt AS m_createdAt, m.updatedAt AS m_updatedAt
		 FROM TextbookMaster AS tm
		 LEFT JOIN TextbookMasterMetric AS m ON m.masterId = tm.id
		 ORDER BY tm.id ASC, m.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	masters := make([]apischema.TextbookMaster, 0)
	for rows.Next() {
		var (
			tm apischema.TextbookMaster
			// LEFT JOIN の相手が居なければ全部 NULL になる
			mID          *int64
			mUnit        *string
			mTotalAmount *int64
			mIsDefault   *bool
			mCreatedAt   *string
			mUpdatedAt   *string
		)
		if err := rows.Scan(
			&tm.ID, &tm.Name, &tm.Publisher, &tm.Edition, &tm.ISBN, &tm.CreatedAt, &tm.UpdatedAt,
			&mID, &mUnit, &mTotalAmount, &mIsDefault, &mCreatedAt, &mUpdatedAt,
		); err != nil {
			return nil, err
		}
		// ORDER BY で同じマスターの行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(masters) == 0 || masters[len(masters)-1].ID != tm.ID {
			tm.CreatedAt = database.ISOFromDatetime(tm.CreatedAt)
			tm.UpdatedAt = database.ISOFromDatetime(tm.UpdatedAt)
			tm.Metrics = make([]apischema.TextbookMasterMetric, 0)
			masters = append(masters, tm)
		}
		if mID != nil {
			last := &masters[len(masters)-1]
			last.Metrics = append(last.Metrics, apischema.TextbookMasterMetric{
				ID:          *mID,
				MasterID:    last.ID,
				Unit:        *mUnit,
				TotalAmount: *mTotalAmount,
				IsDefault:   *mIsDefault,
				CreatedAt:   database.ISOFromDatetime(*mCreatedAt),
				UpdatedAt:   database.ISOFromDatetime(*mUpdatedAt),
			})
		}
	}
	return masters, rows.Err()
}

type textbookHandlers struct {
	store *textbookStore
}

// list は GET /api/textbooks。
func (h *textbookHandlers) list(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	textbooks, err := h.store.listTextbooks(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("textbooks: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, textbooks)
}

// listMasters は GET /api/textbook-masters。ログイン必須だが、中身は利用者によらない。
func (h *textbookHandlers) listMasters(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	snap, err := h.store.masters.Get(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("textbook-masters: %w", err))
		return
	}
	httpx.WriteJSONSnapshot(w, r, snap)
}
