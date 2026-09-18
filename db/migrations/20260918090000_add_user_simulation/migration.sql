-- シミュレーション（sim/）が作る合成ユーザーの管理情報。
-- 実ユーザーは全部 NULL のまま。合成ユーザーだけが simSeq を持つ。
-- 登録日は実際の動線と同じく createdAt を使う（専用の列は持たない）。
--   simSeq          連番。ペルソナ（続き方・主科目など）はこの番号から毎回同じに導く
--   simCohort       steady / fading / sporadic / dropped
--   simDormantFrom  実際に来なくなった日（Asia/Tokyo の日付）
--   simLastActedOn  最後に何か操作した日（同上）
ALTER TABLE `user`
  ADD COLUMN `simSeq` INT NULL,
  ADD COLUMN `simCohort` VARCHAR(32) NULL,
  ADD COLUMN `simDormantFrom` DATE NULL,
  ADD COLUMN `simLastActedOn` DATE NULL,
  ADD UNIQUE INDEX `user_simSeq_key` (`simSeq`);
