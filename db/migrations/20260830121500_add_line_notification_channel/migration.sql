-- Add LINE-specific notification preferences while preserving existing email settings.
ALTER TABLE `NotificationPreference`
  ADD COLUMN `lineMorningEnabled` BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN `lineEveningEnabled` BOOLEAN NOT NULL DEFAULT false;

-- Deliveries are idempotent per channel so email and LINE can both be sent.
ALTER TABLE `NotificationDelivery`
  DROP INDEX `NotificationDelivery_userId_date_slot_key`,
  DROP INDEX `NotificationDelivery_date_slot_idx`,
  ADD COLUMN `channel` VARCHAR(191) NOT NULL DEFAULT 'email',
  ADD INDEX `NotificationDelivery_date_slot_channel_idx`(`date`, `slot`, `channel`),
  ADD UNIQUE INDEX `NotificationDelivery_userId_date_slot_channel_key`(`userId`, `date`, `slot`, `channel`);

CREATE TABLE `LineConnection` (
  `id` INTEGER NOT NULL AUTO_INCREMENT,
  `userId` VARCHAR(191) NOT NULL,
  `lineUserId` VARCHAR(191) NOT NULL,
  `linkedAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updatedAt` DATETIME(3) NOT NULL,
  UNIQUE INDEX `LineConnection_userId_key`(`userId`),
  UNIQUE INDEX `LineConnection_lineUserId_key`(`lineUserId`),
  PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE `LineLinkNonce` (
  `nonce` VARCHAR(255) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX `LineLinkNonce_userId_idx`(`userId`),
  INDEX `LineLinkNonce_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`nonce`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `LineConnection`
  ADD CONSTRAINT `LineConnection_userId_fkey`
  FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE `LineLinkNonce`
  ADD CONSTRAINT `LineLinkNonce_userId_fkey`
  FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
