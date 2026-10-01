-- メールアドレスごとのサインインの試行回数（JUK-93、セキュリティ基準 B4）。
-- Better Auth の回数制限は IP 単位だけなので、IP を変えながら同じアカウントを狙う総当たりは止まらない。
-- デプロイの入れ替えでコンテナが2つ並ぶため、数はメモリではなくここに置く（apps/api/src/sign-in-throttle.ts）。
--
-- 存在しないメールアドレスも同じように数える（数えるかどうかで、登録の有無が分かってしまうため）。
-- 入力されたメールアドレスをそのまま残さないよう、小文字にした SHA-256 で持つ。
-- 窓が終わった行は、サインインを受けたときに少しずつ消す。
CREATE TABLE `SignInAttempt` (
  `emailHash` CHAR(64) NOT NULL,
  `count` INT UNSIGNED NOT NULL,
  `windowStartedAt` DATETIME(3) NOT NULL,
  INDEX `SignInAttempt_windowStartedAt_idx`(`windowStartedAt`),
  PRIMARY KEY (`emailHash`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
