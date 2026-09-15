-- 学習記録・予定を「ユーザー × 日付の範囲」で引けるようにする索引。
--
-- 毎日の通知（sendDailyNotifications）は、全員分の今日の実績・予定を
-- WHERE userId IN (...全員...) AND date >= ? AND date < ? で引く。userId だけの索引では
-- 全員を指定すると絞り込みにならず、表を全行読んでいた（36.9万行の検証で 268ms → 1.75ms）。
--
-- 新しい索引は左端が userId なので、外部キーのために置いていた userId だけの索引は要らなくなる。
-- 同じ ALTER TABLE の中で置き換え、外部キーが索引を失う瞬間を作らない。
ALTER TABLE `StudyLog`
  ADD INDEX `StudyLog_userId_date_idx` (`userId`, `date`),
  DROP INDEX `StudyLog_userId_idx`;

ALTER TABLE `StudyPlan`
  ADD INDEX `StudyPlan_userId_date_idx` (`userId`, `date`),
  DROP INDEX `StudyPlan_userId_idx`;

-- UNIQUE (userId, name) の左端と同じ列だけの索引で、使われることが無い。
ALTER TABLE `Textbook` DROP INDEX `Textbook_userId_idx`;
