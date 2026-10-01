-- アプリが送ったメールの記録（JUK-94、セキュリティ基準 E1）。
-- 確認メールの再送・パスワードの再設定は、宛先を指定すれば誰でも送らせられる。宛先ごとと全体で
-- 送った数を数え、上限を超えたら送らない（apps/api/src/infra/email-limits.ts）。
-- Resend の無料枠（1日100通）を使い切られると、本物の利用者の確認メールも届かなくなるため。
--
-- 宛先はそのまま残さず、小文字にした SHA-256 で持つ。運営者への通知も全体の数に入れるので kind で分ける。
-- 1日より古い行は、送るときに少しずつ消す。
CREATE TABLE `EmailSend` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `recipientHash` CHAR(64) NOT NULL,
  `kind` VARCHAR(32) NOT NULL,
  `sentAt` DATETIME(3) NOT NULL,
  INDEX `EmailSend_recipientHash_sentAt_idx`(`recipientHash`, `sentAt`),
  INDEX `EmailSend_sentAt_idx`(`sentAt`),
  PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
