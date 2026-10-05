-- 運用コマンド（apps/api/cli.go の incident・grant-admin）が変えたことの記録（JUK-138、セキュリティ基準 H4）。
-- 管理画面の操作は構造化ログで Loki に残るが、コマンドは `docker exec` で動くので、出力は実行した人の端末にしか
-- 出ず、コンテナのログに乗らない。止める・権限を変える・2段階認証を戻すといった操作を後から追えるよう、DB に書く。
--
-- 「誰が」は、本番では SSM で EC2 に入った人になる。コンテナの中からはそれが分からないので、ここには
-- 実行したホスト（コンテナ ID）と日時を残し、CloudTrail の StartSession（IAM の利用者・時刻）と突き合わせる。
-- 個人情報を入れすぎないよう、対象はメールアドレスではなく userId で持つ。
-- 退会しても行は残す（外部キーなし）。プライバシーポリシーの「不正利用または紛争の調査に必要な記録が、
-- 必要な期間に限って残る」に当たる。期間は CloudTrail と同じ1年で、コマンドが記録を書くときに古い行を消す。
CREATE TABLE `OpsAuditLog` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `action` VARCHAR(32) NOT NULL,
  `targetId` VARCHAR(191) NULL,
  `detail` JSON NOT NULL,
  `host` VARCHAR(255) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  INDEX `OpsAuditLog_createdAt_idx`(`createdAt`),
  INDEX `OpsAuditLog_targetId_createdAt_idx`(`targetId`, `createdAt`),
  PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
