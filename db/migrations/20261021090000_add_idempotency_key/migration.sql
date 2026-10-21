-- 学習記録の作成（POST /api/study-logs）の冪等キー（学習 2-2、JUK-206）。
-- 保存は成功したのに応答だけ失われ、画面が同じ操作を送り直すと、今は2件になる。画面は操作ごとに
-- Idempotency-Key を付け、送り直しでは同じキーを送る。サーバーはここで「そのキーで保存済みか」を見て、
-- 保存済みなら新しく作らず、1回目の結果を返す。
--
-- 行は記録と同じトランザクションの最初に INSERT する。同じキーの2本目は主キーのロックで1本目のコミットを
-- 待ち、重複（1062）になってから1回目の結果を読む。記録が保存されなければ、この行も一緒に取り消される。
--
-- idemKey と requestHash は ascii_bin にする。utf8mb4_unicode_ci は大文字と小文字を同じとみなすので、
-- 別のキーが同じキーとして扱われてしまう。
-- studyLogId は INSERT の時点ではまだ無いので NULL を許し、記録を作ったあとに埋める。記録を消したら行も消す。
-- isFirstStudyLog は記録の行に残らないので、送り直しに1回目と同じ応答を返すためにここに残す。
--
-- userId には外部キーを付けない。付けると INSERT が user の行に共有ロックを取り、続く初回記録の印
-- （UPDATE user）の排他ロックと、同じ利用者の別のキーの作成どうしでデッドロックになる（10本同時で9本が 1213）。
-- コミットされた行には必ず studyLogId があり、利用者を消すと実績が消え、実績が消えるとこの行も消えるので、
-- userId の外部キーが無くても行は残らない。
CREATE TABLE `IdempotencyKey` (
  `userId` VARCHAR(191) NOT NULL,
  `idemKey` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `requestHash` CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `studyLogId` INT NULL,
  `isFirstStudyLog` BOOLEAN NOT NULL DEFAULT false,
  `createdAt` DATETIME(3) NOT NULL,
  INDEX `IdempotencyKey_studyLogId_idx`(`studyLogId`),
  PRIMARY KEY (`userId`, `idemKey`),
  CONSTRAINT `IdempotencyKey_studyLogId_fkey` FOREIGN KEY (`studyLogId`) REFERENCES `StudyLog`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
