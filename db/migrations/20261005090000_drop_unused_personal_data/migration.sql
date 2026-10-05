-- 目的の無くなった個人情報を消す（JUK-127、セキュリティ基準 G1。JUK-119 もここで片付ける）。
--
-- 課金の移管（受験英語の LINE サポートは別アプリへ移した）で残った表と、ログインを Go へ移した
-- （JUK-115、20261004090000_go_auth）あとの Better Auth の表。どれもアプリは読み書きしていない。
-- Better Auth の表は Node の版へ戻すときのために残していたが、Node のサーバーはもう無い（JUK-109・JUK-121）。
--
-- 2026-10-05 に本番を閲覧用ユーザーで数えて確かめたこと：
--   - Support* の3つ・verification・SignInAttempt は0件
--   - account のパスワードと Google・GitHub の結びつきは、すべて AuthPassword・AuthIdentity に引き継がれている
--     （account にしか無いものは0件。go_auth を当てた後に account へ書かれた行も0件）
--   - twoFactor は1件で、本人は AuthTotp で設定し直し済み。user.twoFactorEnabled は今は AuthTotp から求めている
DROP TABLE `SupportCheckoutInvitation`;
DROP TABLE `SupportSubscription`;
DROP TABLE `SupportLineConnection`;
DROP TABLE `account`;
DROP TABLE `session`;
DROP TABLE `verification`;
DROP TABLE `twoFactor`;
DROP TABLE `SignInAttempt`;

ALTER TABLE `user` DROP COLUMN `twoFactorEnabled`;
