-- AlterTable
ALTER TABLE `StudyPlan` ADD COLUMN `rangeEnd` INTEGER NULL,
    ADD COLUMN `rangeStart` INTEGER NULL,
    ADD COLUMN `rangeUnit` VARCHAR(191) NULL,
    ADD COLUMN `textbookId` INTEGER NULL,
    MODIFY `content` TEXT NULL;

-- CreateTable
CREATE TABLE `Textbook` (
    `id` INTEGER NOT NULL AUTO_INCREMENT,
    `userId` VARCHAR(191) NOT NULL,
    `name` VARCHAR(191) NOT NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    INDEX `Textbook_userId_idx`(`userId`(191)),
    UNIQUE INDEX `Textbook_userId_name_key`(`userId`, `name`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- CreateIndex
CREATE INDEX `StudyPlan_textbookId_idx` ON `StudyPlan`(`textbookId`);

-- AddForeignKey
ALTER TABLE `StudyPlan` ADD CONSTRAINT `StudyPlan_textbookId_fkey` FOREIGN KEY (`textbookId`) REFERENCES `Textbook`(`id`) ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE `Textbook` ADD CONSTRAINT `Textbook_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE;
