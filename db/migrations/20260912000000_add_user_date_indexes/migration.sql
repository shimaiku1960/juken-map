-- 学習予定・実績は「そのユーザーの、ある日付範囲」で引く。
-- userId だけの索引では、そのユーザーの全行を読んでから日付で絞ることになる。
-- 日次通知（cron）は対象ユーザーをまとめて IN で渡すため、userId の索引が選択的でなくなり
-- 全表走査に落ちていた。(userId, date) の複合索引にすると範囲走査で済む。
CREATE INDEX `StudyPlan_userId_date_idx` ON `StudyPlan`(`userId`, `date`);
CREATE INDEX `StudyLog_userId_date_idx` ON `StudyLog`(`userId`, `date`);

-- (userId) 単独の索引は (userId, date) の先頭列と重なるため役目を終える。
-- 外部キーが要求する索引も複合索引が満たすので、残しても書き込みのコストが増えるだけ。
DROP INDEX `StudyPlan_userId_idx` ON `StudyPlan`;
DROP INDEX `StudyLog_userId_idx` ON `StudyLog`;
