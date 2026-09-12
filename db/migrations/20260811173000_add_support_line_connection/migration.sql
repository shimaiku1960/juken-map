-- CreateTable
CREATE TABLE `SupportLineConnection` (
    `id` VARCHAR(191) NOT NULL,
    `userId` VARCHAR(191) NOT NULL,
    `codeHash` VARCHAR(191) NULL,
    `codeExpiresAt` DATETIME(3) NULL,
    `linkedAt` DATETIME(3) NULL,
    `lineDisplayName` VARCHAR(191) NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    UNIQUE INDEX `SupportLineConnection_userId_key`(`userId`),
    UNIQUE INDEX `SupportLineConnection_codeHash_key`(`codeHash`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- AddForeignKey
ALTER TABLE `SupportLineConnection` ADD CONSTRAINT `SupportLineConnection_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
