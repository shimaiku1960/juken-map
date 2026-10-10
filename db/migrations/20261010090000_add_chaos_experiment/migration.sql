-- 本番で障害をわざと起こす実験（カオスエンジニアリング、JUK-171・JUK-173）の記録。
-- API（apps/api/internal/fault）が5秒ごとに実行中の行を読み、対象のリクエストに障害を起こす。
-- 機械の入口（/api/chaos/）が行を作り、管理画面（/admin）からも止められる。
--
-- 終わる時刻（endsAt）は必須で、来たら止める。途中で止めたら stoppedAt と stoppedBy（admin:<userId> か job）を入れる。
-- 気づくまでの時間（MTTD）と直るまでの時間（MTTR）を後で測る起点になるので、終わった行も消さずに残す
-- （週に2〜3行しか増えない）。
CREATE TABLE `ChaosExperiment` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `kind` VARCHAR(32) NOT NULL,
  `route` VARCHAR(191) NOT NULL,
  `rate` DOUBLE NOT NULL,
  `delayMs` INT UNSIGNED NOT NULL DEFAULT 0,
  `statusCode` SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  `startsAt` DATETIME(3) NOT NULL,
  `endsAt` DATETIME(3) NOT NULL,
  `stoppedAt` DATETIME(3) NULL,
  `stoppedBy` VARCHAR(191) NULL,
  INDEX `ChaosExperiment_endsAt_idx`(`endsAt`),
  PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
