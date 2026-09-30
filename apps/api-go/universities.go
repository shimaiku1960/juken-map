package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// 大学の読み取り（JUK-73）。Node の routes/universities.ts と services/university-service.ts にあたる。
//   - GET /api/universities       大学を探す画面の一覧。全員に同じもの
//   - GET /api/universities/{id}  大学詳細。学部・タグと、自分が志望校に登録済みの学部
// 大学・学部・タグの編集は管理画面（Node の /api/admin/*）にある。

// 一覧の応答の形。画面が使うのは大学の列と「学部ごとのタグ名」だけなので、Node と同じくそれだけ返す。
// JSON のバイト列を Node の JSON.stringify と同じにする（下の ETag を Node と揃えるため）ので、
// キーの並びも Node の組み立てと同じにしてある。
type exploreUniversityDTO struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Prefecture string              `json:"prefecture"`
	Type       string              `json:"type"`
	Faculties  []exploreFacultyDTO `json:"faculties"`
}

type exploreFacultyDTO struct {
	Tags []exploreTagDTO `json:"tags"`
}

type exploreTagDTO struct {
	Name string `json:"name"`
}

// 大学一覧は全員に同じもので、変わるのは管理画面でマスターを編集したときだけ。毎回 DB を引くと
// 一番重い API（大学 823 件 × LEFT JOIN 3本）になるので、JSON にした状態でメモリに持つ。
//
// Node は管理画面の編集で自分のキャッシュを捨てているが、その編集は Go には届かない
// （管理 API は JUK-78 で Go へ移すまで Node にある）。そこで期限を Node の10分より短い1分にする。
// 編集が大学を探す画面に出るまで、最大1分かかる。管理 API を Go へ移したら、編集のときに捨てる形にする。
const exploreCacheTTL = time.Minute

// exploreSnapshot は一覧の JSON と、その gzip 版・ETag。
// 圧縮は読み込みのときの1回だけなので、圧縮率を最大にしてよい。リクエストのたびに nginx が
// 80KB を圧縮し直すのを避ける。Node は br も持つが、Go の標準ライブラリに br は無いので gzip だけ。
type exploreSnapshot struct {
	json      []byte
	gzip      []byte
	etag      string
	expiresAt time.Time
}

type universityStore struct {
	db  *sql.DB
	now func() time.Time

	snapshot atomic.Pointer[exploreSnapshot]
	// 期限切れの直後に同時に来たリクエストは、1回の読み込みを待ち合わせる（Node の exploreLoading）。
	loading singleflight.Group
}

func newUniversityStore(db *sql.DB) *universityStore {
	return &universityStore{db: db, now: time.Now}
}

// explore は一覧のスナップショットを返す。期限内なら DB を引かない。
func (st *universityStore) explore(ctx context.Context) (*exploreSnapshot, error) {
	if s := st.snapshot.Load(); s != nil && st.now().Before(s.expiresAt) {
		return s, nil
	}
	v, err, _ := st.loading.Do("explore", func() (any, error) {
		// 待ち合わせている全員のための読み込みなので、最初に来たリクエストが切断しても止めない。
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		s, err := st.loadExplore(loadCtx)
		if err != nil {
			return nil, err
		}
		st.snapshot.Store(s)
		return s, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*exploreSnapshot), nil
}

func (st *universityStore) loadExplore(ctx context.Context) (*exploreSnapshot, error) {
	universities, err := st.listForExplore(ctx)
	if err != nil {
		return nil, err
	}
	body, err := marshalLikeJS(universities)
	if err != nil {
		return nil, err
	}
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := w.Write(body); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	// ETag は JSON の SHA-1 を base64url にしたもの（Node と同じ作り方）。JSON が同じなら値も同じなので、
	// nginx の振り分けで Node と Go を行き来しても、ブラウザが持っている ETag で 304 が返る。
	sum := sha1.Sum(body)
	return &exploreSnapshot{
		json:      body,
		gzip:      gz.Bytes(),
		etag:      `"` + base64.RawURLEncoding.EncodeToString(sum[:]) + `"`,
		expiresAt: st.now().Add(exploreCacheTTL),
	}, nil
}

// marshalLikeJS は JSON.stringify と同じバイト列にする。< > & を書き換えず、末尾に改行を付けない。
func marshalLikeJS(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// listForExplore は大学 → 学部 → タグを LEFT JOIN 1本で取り、入れ子へ詰め直す。
// 行は（大学 × 学部 × タグ）の数だけ並ぶ。
func (st *universityStore) listForExplore(ctx context.Context) ([]exploreUniversityDTO, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, f.id AS facultyId, t.name AS tagName
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 ORDER BY u.name ASC, u.id ASC, f.id ASC, t.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	universities := make([]exploreUniversityDTO, 0)
	var lastFacultyID *int64
	for rows.Next() {
		var (
			u         exploreUniversityDTO
			facultyID *int64
			tagName   *string
		)
		if err := rows.Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &facultyID, &tagName); err != nil {
			return nil, err
		}
		// ORDER BY で同じ大学・同じ学部の行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(universities) == 0 || universities[len(universities)-1].ID != u.ID {
			u.Faculties = make([]exploreFacultyDTO, 0)
			universities = append(universities, u)
			lastFacultyID = nil
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if facultyID == nil {
			continue
		}
		last := &universities[len(universities)-1]
		if lastFacultyID == nil || *lastFacultyID != *facultyID {
			last.Faculties = append(last.Faculties, exploreFacultyDTO{Tags: make([]exploreTagDTO, 0)})
			lastFacultyID = facultyID
		}
		if tagName != nil {
			f := &last.Faculties[len(last.Faculties)-1]
			f.Tags = append(f.Tags, exploreTagDTO{Name: *tagName})
		}
	}
	return universities, rows.Err()
}

// 大学詳細の応答の型は openapi/openapi.yaml から生成した UniversityDetailResponse。
// 学部（FacultyWithTags）は志望校の学部と違い、大学を入れ子にしない。

// findDetail は大学詳細ページ用。学部と、絞り込みに使うタグまで一度に引く。無ければ nil。
func (st *universityStore) findDetail(ctx context.Context, id int64) (*UniversityDetail, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, u.createdAt,
		        f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
		        f.createdAt AS f_createdAt, f.universityId AS f_universityId,
		        t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 WHERE u.id = ?
		 ORDER BY f.id ASC, t.id ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detail *UniversityDetail
	for rows.Next() {
		var (
			u                            UniversityDetail
			fID, fUniversityID, tID      *int64
			fName, fExamDate, fCreatedAt *string
			tName, tCreatedAt            *string
		)
		if err := rows.Scan(
			&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.CreatedAt,
			&fID, &fName, &fExamDate, &fCreatedAt, &fUniversityID,
			&tID, &tName, &tCreatedAt,
		); err != nil {
			return nil, err
		}
		if detail == nil {
			u.CreatedAt = isoFromDatetime(u.CreatedAt)
			u.Faculties = make([]FacultyWithTags, 0)
			detail = &u
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if fID == nil {
			continue
		}
		if n := len(detail.Faculties); n == 0 || detail.Faculties[n-1].ID != *fID {
			detail.Faculties = append(detail.Faculties, FacultyWithTags{
				ID:           *fID,
				Name:         *fName,
				ExamDate:     isoFromDatetime(*fExamDate),
				CreatedAt:    isoFromDatetime(*fCreatedAt),
				UniversityID: *fUniversityID,
				Tags:         make([]Tag, 0),
			})
		}
		if tID != nil {
			f := &detail.Faculties[len(detail.Faculties)-1]
			f.Tags = append(f.Tags, Tag{ID: *tID, Name: *tName, CreatedAt: isoFromDatetime(*tCreatedAt)})
		}
	}
	return detail, rows.Err()
}

// listGoalFacultyIDs は志望校として登録済みの学部 ID。大学詳細で「登録済み」を出し分けるのに使う。
func (st *universityStore) listGoalFacultyIDs(ctx context.Context, userID string) ([]int64, error) {
	rows, err := st.db.QueryContext(ctx,
		"SELECT facultyId FROM FinalGoal WHERE userId = ? ORDER BY facultyId ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type universityHandlers struct {
	store *universityStore
}

// list は GET /api/universities。ブラウザには毎回確かめさせ（no-cache）、変わっていなければ 304 で
// 本文を省く。ログインが要る応答なので共有キャッシュには置かせない（private）。
func (h *universityHandlers) list(w http.ResponseWriter, r *http.Request, _ *session) {
	snap, err := h.store.explore(r.Context())
	if err != nil {
		internalError(w, r, fmt.Errorf("universities: %w", err))
		return
	}
	hdr := w.Header()
	hdr.Set("Cache-Control", "private, no-cache")
	hdr.Set("ETag", snap.etag)
	// 同じ URL でも Accept-Encoding で本文の形が変わる、と途中のキャッシュに伝える。
	hdr.Set("Vary", "Accept-Encoding")
	if matchesETag(r.Header.Get("If-None-Match"), snap.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hdr.Set("Content-Type", "application/json; charset=utf-8")
	body := snap.json
	// Content-Encoding を付けて返すと、nginx の gzip は圧縮し直さない。
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		hdr.Set("Content-Encoding", "gzip")
		body = snap.gzip
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// detail は GET /api/universities/{id}。無い大学は 404（Node と同じ文言）。
func (h *universityHandlers) detail(w http.ResponseWriter, r *http.Request, s *session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	university, err := h.store.findDetail(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("universities/:id: %w", err))
		return
	}
	if university == nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	// 画面は「この学部は登録済みか」を出し分けるので、同じ応答に含める。
	registered, err := h.store.listGoalFacultyIDs(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("universities/:id goals: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, UniversityDetailResponse{University: *university, RegisteredFacultyIds: registered})
}

// matchesETag は If-None-Match（カンマ区切りで複数並べられる）に etag が含まれるか。
// 途中で圧縮し直されると ETag は弱い形（W/"..."）で戻ってくるので、W/ を外して比べる（Node と同じ）。
func matchesETag(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	for _, v := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(v), "W/") == etag {
			return true
		}
	}
	return false
}

// acceptsGzip は Accept-Encoding が gzip（または *）を受け付けるか。"gzip;q=0" は「受け付けない」。
// Node の pickEncoding から br を除いたもの。
func acceptsGzip(acceptEncoding string) bool {
	for _, part := range strings.Split(acceptEncoding, ",") {
		name, params, _ := strings.Cut(strings.ToLower(strings.TrimSpace(part)), ";")
		name = strings.TrimSpace(name)
		if name != "gzip" && name != "*" {
			continue
		}
		rejected := false
		for _, p := range strings.Split(params, ";") {
			if q, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok && isZeroQ(q) {
				rejected = true
			}
		}
		if !rejected {
			return true
		}
	}
	return false
}

// isZeroQ は q の値が 0 か。Node は Number(q) === 0 で見ていて、空文字（"q="）も 0 になる。
func isZeroQ(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return true
	}
	f, err := strconv.ParseFloat(q, 64)
	return err == nil && f == 0
}
