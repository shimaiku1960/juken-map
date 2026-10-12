# DB

バックエンドの学習計画（`docs/learning/README.md`）の「1. DB」で学んだことのまとめ。回ごとに節を足していく。

## 1-1 インデックスと EXPLAIN（2026-10-12）

### 学んだこと

- **`EXPLAIN` は計画、`EXPLAIN ANALYZE` は実際。** `EXPLAIN` は SQL を動かさずに「どの表を、どの索引で、何行くらい読むつもりか」を出す。`EXPLAIN ANALYZE` は実際に動かして、段ごとにかかった時間と読んだ行数を出す。見積もり（`cost=… rows=…`）と実際（`actual … rows=…`）は大きく外れることがある
- **`type` で読み方が分かる。** `ALL` は全件、`ref` は索引で等しい値を探す、`range` は索引で範囲を探す、`eq_ref` は JOIN の相手を一意の索引で1行だけ引く
- **`key_len` で、複合索引の何列目まで使えたかが分かる。** `(userId, date)` の索引で `date` の条件を足すと 766 → 773 に増えた（`date` の7バイト分）
- **`Extra` の `Using filesort` は「索引の順では足りず、読んだあとに並べ替えた」印。** 原因は列が足りないことだけでなく、並べる向きが索引と合わないことでも起きる
- **InnoDB の二次索引には、末尾に主キーが付いている。** `StudyLog_userId_date_idx` は定義上 `(userId, date)` だが、中身は `(userId, date, id)` の並びなので、`ORDER BY date DESC, id DESC` まで索引の順で読める
- **時間は2回目以降で比べる。** 1回目はデータがバッファプール（メモリ）に無く、ディスクから読むので遅い。同じ SQL で 61.3ms → 6.28ms（約10倍）だった
- **索引が効くのは、ごく一部に絞れるとき。** 全体の大半が条件に合うなら、索引で探すより全部読むほうが速く、MySQL は索引を使わない
- **`JOIN` と `LEFT JOIN` の違いは、相手が見つからない行を残すかどうか。** 毎日の通知の SQL は、通知設定が無い人は対象外なので `JOIN`、LINE を連携していない人にもメールは送るので `LineConnection` 側を `LEFT JOIN` にしている

### 数字で見た結果

#### 学習記録（StudyLog）の一覧

| 実験 | SQL の形 | `type` / `Extra` | 読んだ行 | 時間 |
| -- | -- | -- | -- | -- |
| 1〜6 | 索引の無い列（`memo`）で絞る | `ALL` | 1080万行（全件） | 2445ms |
| 1〜6 | 索引のある列（`userId`）で絞る | `ref`・`Using index` | 1746行 | 1.14ms（約2100倍速い） |
| 1〜6 | `userId` に `date >= '2026-09-01'` を足す | `range`、`key_len` 766 → 773 | 見積もり 1746 → 279 | — |
| 7 | `ORDER BY date DESC, id ASC LIMIT 1000`（本番の一覧 `study.go:119` と同じ形） | `Using filesort` | — | — |
| 8 | `id DESC` にそろえる | filesort が消え、`Backward index scan` | — | — |
| 9 | 7 を `EXPLAIN ANALYZE`（起動直後の1回目） | 読むのに 59.7ms、並べ替え約1ms | 1746行 | 61.3ms |
| 10 | 9 と同じ SQL の2回目 | — | 1746行 | 6.28ms |
| 11 | 8 を `EXPLAIN ANALYZE` | Sort の段が消える。LIMIT で途中で止まる | 1000行 | 5.68ms |

`memo` で絞った実験では、見積もり（103万行）と実際に返った行（35行）が大きく外れた。

#### 毎日の通知の宛先（`notifications.go:266`）

| 実験 | 見たもの | 結果 |
| -- | -- | -- |
| 12 | `EXPLAIN` の `np`（NotificationPreference）の行 | `type: ALL`、`key: NULL`、見積もり 16,264行・`filtered` 19%、`Using temporary; Using filesort` |
| 13 | `EXPLAIN ANALYZE`（2回目） | 全体 **44.9ms**、結果 11,597人 |

実験13の段ごとの内訳：

| 段 | 終わった時刻 | 行数 |
| -- | -- | -- |
| np を全件読む（Table scan） | 2.29ms | 16,590 |
| 朝の通知がオンの人に絞る（Filter） | 3.25ms | 11,597 |
| 1人ずつ user を主キーで引く（`loops=11597`） | 27ms | 11,597 |
| 1人ずつ LINE 連携を引く | 31.4ms | 11,597 |
| `u.id` の順に並べる（Sort） | 44.9ms | 11,597 |

- 全件読みそのものは 2.29ms で、時間の大半は「1人ずつ引く」（約28ms）と並べ替え（約13ms）
- MySQL の見積もりは「19%が合う」（3,090人）だったが、実際は約70%（11,597人）。オン/オフだけの列には統計が無く、決まった割合で推測するため
- 7割が合う条件なので、`morningEnabled` に索引を足しても使われない見込みが高い

### 変えたこと

本番のコードは変えていない。

- 学習記録の一覧（実験7〜11）：`id DESC` にそろえると filesort が消えるが、短くなるのは約0.6ms。データが少ない今は変えない。「より最適化したいなら」変えてよい
- 毎日の通知の宛先（実験12〜13）：1日2回のバッチで 45ms なので変えない

計画にあった「`(userId, date)` を一度外して比べる」は行っていない。代わりに、索引の無い列（`memo`）と索引のある列（`userId`）で同じ表を比べた。

### 面接で話す要点（STAR）

**日本語**

- **S（状況）**：学習記録アプリで、利用者ごとの記録の一覧と、毎日の通知の宛先を DB から取っている。記録の表は検証用に1080万行まで増やした
- **T（課題）**：速さを「なんとなく速い」ではなく、EXPLAIN で読み方を説明し、数字で比べられるようにしたかった
- **A（行動）**：索引の有無で `type` と読んだ行数を比べ、全件読みの 2445ms が索引で 1.14ms になることを確かめた。一覧の SQL では `Using filesort` が出たので、InnoDB の二次索引の末尾に主キーが付くことを踏まえて並べる向きをそろえ、filesort が消えて読む行が 1746 → 1000 に減ることを `EXPLAIN ANALYZE` で確かめた。通知の宛先の SQL は全件読みだったが、実際に測ると 45ms で、時間の大半は全件読みではなく JOIN の1行ずつの引き当てと並べ替えだった
- **R（結果）**：どちらの SQL も「今は変えない」と判断した。改善幅（約0.6ms）や実行頻度（1日2回）に対して、変える理由が無いため。「遅そうに見える計画」と「実際に遅いか」を分けて判断できるようになった

**English**

- **S**: In my study-log app, the API reads each user's log list and the daily notification recipients from MySQL. I grew the log table to 10.8M rows for testing.
- **T**: I wanted to explain query performance with EXPLAIN output and measured numbers, not intuition.
- **A**: I compared access types with and without an index: a full scan took 2445ms, while an index lookup took 1.14ms. For the list query, EXPLAIN showed `Using filesort`. Knowing that InnoDB secondary indexes include the primary key at the end, I aligned the sort direction, which removed the filesort and cut rows read from 1746 to 1000 (confirmed with `EXPLAIN ANALYZE`). The notification query used a full table scan, but it actually took only 45ms, and most of that time went to per-row join lookups and sorting, not the scan itself.
- **R**: I decided not to change either query, since the gain (about 0.6ms) and the run frequency (twice a day) didn't justify it. I learned to separate "a plan that looks slow" from "a query that is actually slow."
