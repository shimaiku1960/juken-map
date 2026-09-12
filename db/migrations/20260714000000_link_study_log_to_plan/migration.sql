-- AlterTable
ALTER TABLE `StudyLog` ADD COLUMN `studyPlanId` INTEGER NULL;

-- CreateIndex
CREATE UNIQUE INDEX `StudyLog_studyPlanId_key` ON `StudyLog`(`studyPlanId`);

-- AddForeignKey
ALTER TABLE `StudyLog` ADD CONSTRAINT `StudyLog_studyPlanId_fkey` FOREIGN KEY (`studyPlanId`) REFERENCES `StudyPlan`(`id`) ON DELETE SET NULL ON UPDATE CASCADE;
