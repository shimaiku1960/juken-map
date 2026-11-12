-- シミュレーション（sim/）の合成ユーザーの印を、user の列から 1対1 の表に移す（JUK-228）。
-- user.simSeq などに値が入るのは合成ユーザーだけで、実ユーザーの行では4列とも NULL になっている。
-- 表に分けると、この表に行がある利用者だけがシミュレーション用になる。
--
-- docs/migrations.md の決まり1（足す → 移す → 消す）の「足す」。新しい表を作るだけで、既にある表は変えない。
--   このデプロイ：アプリは user の列とこの表の両方に書き、user の列を読む
--   次のデプロイ：user の列からこの表を埋め、アプリはこの表だけを読み書きする
--   その次      ：user の sim の列と索引を消す
--
--   userId       主キーにして 1人1行にする（1対1）。user を消せば一緒に消える
--   seq          連番。ペルソナ（続き方・主科目など）はこの番号から毎回同じに導く
--   cohort       steady / fading / sporadic / dropped
--   dormantFrom  実際に来なくなった日（Asia/Tokyo の日付）
--   lastActedOn  最後に何か操作した日（同上）
CREATE TABLE `UserSimulation` (
  `userId` VARCHAR(191) NOT NULL,
  `seq` INT NOT NULL,
  `cohort` VARCHAR(32) NOT NULL,
  `dormantFrom` DATE NULL,
  `lastActedOn` DATE NULL,
  PRIMARY KEY (`userId`),
  UNIQUE INDEX `UserSimulation_seq_key` (`seq`),
  CONSTRAINT `UserSimulation_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
