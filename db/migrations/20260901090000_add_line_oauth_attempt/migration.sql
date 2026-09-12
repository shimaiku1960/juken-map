CREATE TABLE `LineOAuthAttempt` (
  `state` VARCHAR(255) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `nonce` VARCHAR(255) NOT NULL,
  `codeVerifier` VARCHAR(255) NOT NULL,
  `redirectUri` VARCHAR(500) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX `LineOAuthAttempt_userId_idx`(`userId`),
  INDEX `LineOAuthAttempt_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`state`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `LineOAuthAttempt`
  ADD CONSTRAINT `LineOAuthAttempt_userId_fkey`
  FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
