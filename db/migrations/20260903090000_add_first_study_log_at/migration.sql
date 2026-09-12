ALTER TABLE `user`
  ADD COLUMN `firstStudyLogAt` DATETIME(3) NULL,
  ADD COLUMN `analyticsSignUpTrackedAt` DATETIME(3) NULL;

-- 導入前の既存ユーザーを新規登録として誤計上しない。
UPDATE `user` SET `analyticsSignUpTrackedAt` = CURRENT_TIMESTAMP(3);
