package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/textbookmaster"
)

// 管理者ページのマスター編集のうち、参考書マスター（/api/admin/textbook-masters）。
// 共通の部品と全体の決まりは admin_masters.go。

// ---- 入口 ----

// listTextbookMasters は GET /api/admin/textbook-masters。
func (h *adminMasterHandlers) listTextbookMasters(w http.ResponseWriter, r *http.Request, _ *session) {
	q, issue := readMasterSearchQuery(parseQuery(r.URL.RawQuery))
	if issue != nil {
		issue.write(w)
		return
	}
	masters, err := h.store.listTextbookMasters(r.Context(), q)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin textbook masters: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, masters)
}

// createTextbookMaster は POST /api/admin/textbook-masters。
func (h *adminMasterHandlers) createTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	input, in := readTextbookMasterInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.createTextbookMaster(r.Context(), input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin create textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "TextbookMaster", outcome.value.ID, "after", outcome.value)
	writeJSON(w, http.StatusCreated, outcome.value)
}

// updateTextbookMaster は PATCH /api/admin/textbook-masters/{id}。
func (h *adminMasterHandlers) updateTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readTextbookMasterInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.updateTextbookMaster(r.Context(), id, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin update textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "TextbookMaster", id, "before", outcome.value.before, "after", outcome.value.after)
	writeJSON(w, http.StatusOK, outcome.value.after)
}

// deleteTextbookMaster は DELETE /api/admin/textbook-masters/{id}。
func (h *adminMasterHandlers) deleteTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteTextbookMaster(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin delete textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "TextbookMaster", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 入力 ----

type textbookMasterInput struct {
	name               string
	publisher, edition *string
	isbn               string
	metrics            []apischema.AdminTextbookMasterMetric
}

// record は持ち主に渡す形にする。
func (in textbookMasterInput) record() textbookmaster.Input {
	metrics := make([]textbookmaster.Metric, len(in.metrics))
	for i, m := range in.metrics {
		metrics[i] = textbookmaster.Metric(m)
	}
	return textbookmaster.Input{Name: in.name, Publisher: in.publisher, Edition: in.edition, Isbn: in.isbn, Metrics: metrics}
}

// readTextbookMasterInput は textbookMasterInputSchema。
func readTextbookMasterInput(body any) (textbookMasterInput, *objectInput) {
	in := readObject(body)
	var v textbookMasterInput
	v.name = in.string("name", stringRule{
		trimFirst: true,
		min:       1, minMessage: "参考書名を入力してください",
		max: 150, maxMessage: "参考書名は150文字以内で入力してください",
	})
	v.publisher = readOptionalText(in, "publisher", 100)
	v.edition = readOptionalText(in, "edition", 50)
	// isbn は z.string() を読んでから整え（transform）、整えた後の形を確かめる（refine）。
	v.isbn = normalizeISBN(in.string("isbn", stringRule{}))
	if in.issue == nil && !isbnPattern.MatchString(v.isbn) {
		in.addIssue("invalid_isbn", "isbn", "ISBN は10桁か13桁で入力してください")
	}
	v.metrics = readMetrics(in)
	return v, in
}

// readOptionalText は Node の optionalText(max)：z.string().trim().max(max).nullable().optional() で、
// 空（キーが無い・null・削ったら空）なら null。
func readOptionalText(in *objectInput, key string, max int) *string {
	o := in.optionalString(key, stringRule{trimFirst: true, max: max, maxMessage: fmt.Sprintf("%d文字以内で入力してください", max)}, true)
	if o.value == nil || *o.value == "" {
		return nil
	}
	return o.value
}

// isbnPattern は ISBN-13（数字13桁）か ISBN-10（数字9桁＋数字か X）。
var isbnPattern = regexp.MustCompile(`^(\d{13}|\d{9}[\dX])$`)

// normalizeISBN は Node と同じく、ハイフンと空白（JavaScript の \s）を取り除いて大文字にする。
// 既存の ISBN は数字だけで入っている。大文字にするのは ISBN-10 の最後の x のため
// （JavaScript の toUpperCase と Go の ToUpper は一部の文字で結果が違うが、どちらも isbnPattern に合わない）。
func normalizeISBN(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '-' || isJSWhitespace(r) {
			return -1
		}
		return r
	}, s)
	return strings.ToUpper(s)
}

// readMetrics は総量の候補：z.array(metric).min(1).max(6).refine(単位が重ならない).refine(既定がちょうど1つ)。
// 要素 → min・max → refine の順に確かめる。
func readMetrics(in *objectInput) []apischema.AdminTextbookMasterMetric {
	items := in.array("metrics", "総量を1つ以上入力してください")
	if in.issue != nil {
		return nil
	}
	metrics := make([]apischema.AdminTextbookMasterMetric, 0, len(items))
	for i, item := range items {
		m := readObjectAt(item, in.field("metrics")+"."+strconv.Itoa(i))
		metric := apischema.AdminTextbookMasterMetric{
			Unit: m.string("unit", stringRule{checks: []stringCheck{{
				ok:   func(s string) bool { return apischema.TextbookMasterInputMetricsUnit(s).Valid() },
				code: "invalid_range_unit", message: "単位を選んでください",
			}}}),
			// z.number({ error: "総量を入力してください" }).int(…).min(1, …).max(100000, …)。
			// 整数に限ると min(1) は positive と同じ（0.5 は int で先に弾かれる）。
			TotalAmount: int(m.number("totalAmount", numberRule{
				typeMessage: "総量を入力してください",
				int:         true, intMessage: "総量は整数で入力してください",
				positive: true, positiveMessage: "総量は1以上で入力してください",
				max: 100_000, maxMessage: "総量は100000以下で入力してください",
			})),
			IsDefault: m.boolean("isDefault"),
		}
		if m.issue != nil {
			in.take(m)
			return nil
		}
		metrics = append(metrics, metric)
	}
	if len(metrics) > textbookMetricsMax {
		in.addIssue("too_big", "metrics", "Too big: expected array to have <=6 items")
		return nil
	}
	units := make(map[string]bool, len(metrics))
	defaults := 0
	for _, m := range metrics {
		if units[m.Unit] {
			in.addIssue("duplicate_units", "metrics", "同じ単位が重なっています")
			return nil
		}
		units[m.Unit] = true
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		in.addIssue("default_unit_required", "metrics", "既定の単位を1つ選んでください")
		return nil
	}
	return metrics
}

// ---- DB ----

// selectAdminTextbookMasters は参考書マスターを、総量の候補と利用者の参考書の数をつけて引く（先頭200件まで）。
func selectAdminTextbookMasters(ctx context.Context, db database.Runner, where string, args ...any) ([]apischema.AdminTextbookMaster, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn,
		        (SELECT COUNT(*) FROM Textbook t WHERE t.masterId = tm.id) AS textbookCount,
		        m.unit, m.totalAmount, m.isDefault
		 FROM (SELECT * FROM TextbookMaster tm `+where+` ORDER BY tm.id ASC LIMIT `+strconv.Itoa(adminTextbookMastersLimit)+`) AS tm
		 LEFT JOIN TextbookMasterMetric m ON m.masterId = tm.id
		 ORDER BY tm.id ASC, m.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	masters := []apischema.AdminTextbookMaster{}
	for rows.Next() {
		var (
			tm          apischema.AdminTextbookMaster
			unit        *string
			totalAmount *int
			isDefault   *bool
		)
		if err := rows.Scan(&tm.ID, &tm.Name, &tm.Publisher, &tm.Edition, &tm.Isbn, &tm.TextbookCount, &unit, &totalAmount, &isDefault); err != nil {
			return nil, err
		}
		// 行は（参考書 × 総量の候補）の数だけ並ぶ。同じ参考書の行は隣り合うので、直前と比べて束ねる。
		if n := len(masters); n == 0 || masters[n-1].ID != tm.ID {
			tm.Metrics = []apischema.AdminTextbookMasterMetric{}
			masters = append(masters, tm)
		}
		if unit != nil {
			last := &masters[len(masters)-1]
			last.Metrics = append(last.Metrics, apischema.AdminTextbookMasterMetric{Unit: *unit, TotalAmount: *totalAmount, IsDefault: *isDefault})
		}
	}
	return masters, rows.Err()
}

func (st *sqlAdminMasterStore) listTextbookMasters(ctx context.Context, q string) ([]apischema.AdminTextbookMaster, error) {
	if q == "" {
		return selectAdminTextbookMasters(ctx, st.db, "")
	}
	pattern := "%" + escapeLike(q) + "%"
	return selectAdminTextbookMasters(ctx, st.db, "WHERE tm.name LIKE ? OR tm.publisher LIKE ? OR tm.isbn LIKE ?", pattern, pattern, pattern)
}

func (st *sqlAdminMasterStore) createTextbookMaster(ctx context.Context, in textbookMasterInput) (masterOutcome[apischema.AdminTextbookMaster], error) {
	m, err := textbookmaster.Create(ctx, st.db, in.record(), nowMillis())
	return masterOutcomeOf(adminTextbookMaster(m), err, st.textbookMastersChanged)
}

func (st *sqlAdminMasterStore) updateTextbookMaster(ctx context.Context, id int64, in textbookMasterInput) (masterOutcome[masterChange[apischema.AdminTextbookMaster]], error) {
	c, err := textbookmaster.Update(ctx, st.db, id, in.record(), nowMillis())
	change := masterChange[apischema.AdminTextbookMaster]{before: adminTextbookMaster(c.Before), after: adminTextbookMaster(c.After)}
	return masterOutcomeOf(change, err, st.textbookMastersChanged)
}

func (st *sqlAdminMasterStore) deleteTextbookMaster(ctx context.Context, id int64) (masterOutcome[apischema.AdminTextbookMaster], error) {
	m, err := textbookmaster.Delete(ctx, st.db, id)
	return masterOutcomeOf(adminTextbookMaster(m), err, st.textbookMastersChanged)
}

// adminTextbookMaster は持ち主の行を応答の形にする（総量の候補は項目の並びが同じなので型の変換だけ）。
func adminTextbookMaster(m textbookmaster.Master) apischema.AdminTextbookMaster {
	metrics := make([]apischema.AdminTextbookMasterMetric, len(m.Metrics))
	for i, metric := range m.Metrics {
		metrics[i] = apischema.AdminTextbookMasterMetric(metric)
	}
	return apischema.AdminTextbookMaster{
		Edition: m.Edition, ID: m.ID, Isbn: m.Isbn, Metrics: metrics, Name: m.Name, Publisher: m.Publisher, TextbookCount: m.TextbookCount,
	}
}
