-- CreateTable
CREATE TABLE `SupportCheckoutInvitation` (
    `id` VARCHAR(191) NOT NULL,
    `tokenHash` VARCHAR(191) NOT NULL,
    `email` VARCHAR(191) NOT NULL,
    `expiresAt` DATETIME(3) NOT NULL,
    `usedAt` DATETIME(3) NULL,
    `guardianConsentConfirmedAt` DATETIME(3) NULL,
    `checkoutSessionId` VARCHAR(191) NULL,
    `userId` VARCHAR(191) NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    UNIQUE INDEX `SupportCheckoutInvitation_tokenHash_key`(`tokenHash`),
    UNIQUE INDEX `SupportCheckoutInvitation_checkoutSessionId_key`(`checkoutSessionId`),
    INDEX `SupportCheckoutInvitation_email_idx`(`email`),
    INDEX `SupportCheckoutInvitation_userId_idx`(`userId`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- CreateTable
CREATE TABLE `SupportSubscription` (
    `id` VARCHAR(191) NOT NULL,
    `userId` VARCHAR(191) NOT NULL,
    `stripeCustomerId` VARCHAR(191) NOT NULL,
    `stripeSubscriptionId` VARCHAR(191) NOT NULL,
    `status` VARCHAR(191) NOT NULL,
    `trialUsed` BOOLEAN NOT NULL DEFAULT false,
    `trialEndsAt` DATETIME(3) NULL,
    `currentPeriodEndsAt` DATETIME(3) NULL,
    `cancelAtPeriodEnd` BOOLEAN NOT NULL DEFAULT false,
    `canceledAt` DATETIME(3) NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    UNIQUE INDEX `SupportSubscription_userId_key`(`userId`),
    UNIQUE INDEX `SupportSubscription_stripeCustomerId_key`(`stripeCustomerId`),
    UNIQUE INDEX `SupportSubscription_stripeSubscriptionId_key`(`stripeSubscriptionId`),
    INDEX `SupportSubscription_status_idx`(`status`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- AddForeignKey
ALTER TABLE `SupportCheckoutInvitation` ADD CONSTRAINT `SupportCheckoutInvitation_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE `SupportSubscription` ADD CONSTRAINT `SupportSubscription_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
