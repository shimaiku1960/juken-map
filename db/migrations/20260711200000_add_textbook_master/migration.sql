CREATE TABLE `TextbookMaster` (
    `id` INTEGER NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(191) NOT NULL,
    `publisher` VARCHAR(191) NULL,
    `edition` VARCHAR(191) NULL,
    `isbn` VARCHAR(191) NOT NULL,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    UNIQUE INDEX `TextbookMaster_isbn_key`(`isbn`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE `TextbookMasterMetric` (
    `id` INTEGER NOT NULL AUTO_INCREMENT,
    `masterId` INTEGER NOT NULL,
    `unit` VARCHAR(191) NOT NULL,
    `totalAmount` INTEGER NOT NULL,
    `isDefault` BOOLEAN NOT NULL DEFAULT false,
    `createdAt` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updatedAt` DATETIME(3) NOT NULL,

    INDEX `TextbookMasterMetric_masterId_idx`(`masterId`),
    UNIQUE INDEX `TextbookMasterMetric_masterId_unit_key`(`masterId`, `unit`),
    PRIMARY KEY (`id`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

ALTER TABLE `Textbook`
    ADD COLUMN `masterId` INTEGER NULL,
    ADD INDEX `Textbook_masterId_idx`(`masterId`);

ALTER TABLE `Textbook`
    ADD CONSTRAINT `Textbook_masterId_fkey`
    FOREIGN KEY (`masterId`) REFERENCES `TextbookMaster`(`id`)
    ON DELETE SET NULL ON UPDATE CASCADE;

ALTER TABLE `TextbookMasterMetric`
    ADD CONSTRAINT `TextbookMasterMetric_masterId_fkey`
    FOREIGN KEY (`masterId`) REFERENCES `TextbookMaster`(`id`)
    ON DELETE CASCADE ON UPDATE CASCADE;

INSERT INTO `TextbookMaster`
    (`id`, `name`, `publisher`, `edition`, `isbn`, `updatedAt`)
VALUES
    (1, '英単語ターゲット1900', '旺文社', '6訂版', '9784010346464', CURRENT_TIMESTAMP(3)),
    (2, '改訂版 チャート式 基礎からの数学I+A', '数研出版', '改訂版', '9784410105791', CURRENT_TIMESTAMP(3)),
    (3, '新課程 チャート式 基礎からの数学II+B', '数研出版', '新課程版', '9784410105883', CURRENT_TIMESTAMP(3)),
    (4, '新課程 チャート式 基礎からの数学III+C', '数研出版', '新課程版', '9784410105951', CURRENT_TIMESTAMP(3));

INSERT INTO `TextbookMasterMetric`
    (`masterId`, `unit`, `totalAmount`, `isDefault`, `updatedAt`)
VALUES
    (1, 'number', 1900, true, CURRENT_TIMESTAMP(3)),
    (1, 'page', 528, false, CURRENT_TIMESTAMP(3)),
    (1, 'part', 3, false, CURRENT_TIMESTAMP(3)),
    (1, 'section', 19, false, CURRENT_TIMESTAMP(3)),
    (2, 'page', 696, true, CURRENT_TIMESTAMP(3)),
    (3, 'page', 664, true, CURRENT_TIMESTAMP(3)),
    (4, 'page', 736, true, CURRENT_TIMESTAMP(3));
