-- CreateTable
CREATE TABLE `StudyLog` (
    `id` INTEGER NOT NULL AUTO_INCREMENT,
    `userId` VARCHAR(191) NOT NULL,
    `date` DATETIME(3) NOT NULL,
    `subject` VARCHAR(191) NULL,
    `minutes` INTEGER NOT NULL,
    `textbookId` INTEGER NULL,
    `rangeStart` INTEGER NULL,
    `rangeEnd` INTEGER NULL,
    `rangeUnit` VARCHAR(191) NULL,
    `memo` TEXT NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    INDEX `StudyLog_userId_idx`(`userId`(191)),
    INDEX `StudyLog_textbookId_idx`(`textbookId`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- AddForeignKey
ALTER TABLE `StudyLog` ADD CONSTRAINT `StudyLog_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE `StudyLog` ADD CONSTRAINT `StudyLog_textbookId_fkey` FOREIGN KEY (`textbookId`) REFERENCES `Textbook`(`id`) ON DELETE SET NULL ON UPDATE CASCADE;
