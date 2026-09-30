-- 処理済みの LINE Webhook イベント（JUK-79、セキュリティ基準 C1）。
-- LINE は応答が返らなかったときなどに同じイベントを再送する（deliveryContext.isRedelivery）。
-- イベントごとの webhookEventId を先にここへ入れ、PRIMARY KEY で2回目を弾いて二重に処理しない。
-- 再送は長くても1日ほどなので、古い行は Webhook を受けたときに少しずつ消す（apps/api-go/line.go）。
CREATE TABLE `LineWebhookEvent` (
  `webhookEventId` VARCHAR(64) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX `LineWebhookEvent_createdAt_idx`(`createdAt`),
  PRIMARY KEY (`webhookEventId`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
