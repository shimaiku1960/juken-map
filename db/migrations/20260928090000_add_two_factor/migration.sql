-- 管理者の2段階認証（Better Auth の twoFactor プラグイン、認証アプリの TOTP＋予備コード）。
-- user.twoFactorEnabled と twoFactor 表はプラグインの決まった形（列名もプラグインが決める）。
-- secret と backupCodes は BETTER_AUTH_SECRET で暗号化して入る。
--
-- session.twoFactorVerified はこのアプリで足した列。twoFactor はメール＋パスワードの
-- ログインにしかかからず、Google / GitHub のログインは素通りするので、「そのセッションが
-- 2段階認証を通して作られたか」をセッションの側に持つ。管理 API はこれが TRUE の
-- セッションだけを通す（apps/api/src/context.ts の requireAdmin）。
ALTER TABLE `user`
  ADD COLUMN `twoFactorEnabled` BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE `session`
  ADD COLUMN `twoFactorVerified` BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE `twoFactor` (
    `id` VARCHAR(191) NOT NULL,
    `secret` TEXT NOT NULL,
    `backupCodes` TEXT NOT NULL,
    `userId` VARCHAR(191) NOT NULL,
    `verified` BOOLEAN NOT NULL DEFAULT true,
    `failedVerificationCount` INT NOT NULL DEFAULT 0,
    `lockedUntil` DATETIME(3) NULL,

    INDEX `twoFactor_userId_idx`(`userId`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `twoFactor` ADD CONSTRAINT `twoFactor_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
