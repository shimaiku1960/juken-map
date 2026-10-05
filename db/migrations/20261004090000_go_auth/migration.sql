-- ログインを Better Auth（Node）から Go の自作へ移す（JUK-115）。判定の基準は dev-standards の
-- targets/10_authentication.md（認証 基準 v1.0）。項目の記号（C2 など）はその文書のもの。
--
-- Better Auth の表（session・account・verification・twoFactor）と SignInAttempt はまだ消さない。
-- 切り替えた後に Node の版へ戻すことになっても、Better Auth がそのまま読めるようにしておくため。
-- 消すのは、Go で動くのを本番で確かめてから別のマイグレーションで行う。
--
-- 引き継ぐのはパスワードと外部ログインの結びつきだけ。セッションは Cookie の形が変わるので
-- 引き継がず、全員がログインし直す。2段階認証の秘密は Better Auth の鍵で暗号化されていて
-- ここでは読めないので、設定し直してもらう（管理者は本人だけ）。

-- セッション（C1〜C5）。Cookie で運ぶのは 256 ビットの乱数のトークンで、ここには SHA-256 だけを置く（C2）。
-- DB の値が漏れても、そこから Cookie は作れない。
--   id                  一覧・個別の取り消しに使う番号。トークンではないので画面に出してよい
--   expiresAt           ログインからの上限（一般30日・管理者24時間）。使っても延ばさない（C3）
--   idleTimeoutSeconds  使わないときの期限。管理者だけ 3600。NULL は置かない
--   lastUsedAt          最後に使った時刻。使わないときの期限と、端末の一覧に使う
--   mfaVerifiedAt       2段階認証を通して作ったセッションならその時刻（G3）。管理 API はこれを求める
CREATE TABLE `AuthSession` (
  `id` CHAR(32) NOT NULL,
  `tokenHash` BINARY(32) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  `idleTimeoutSeconds` INT UNSIGNED NULL,
  `lastUsedAt` DATETIME(3) NOT NULL,
  `mfaVerifiedAt` DATETIME(3) NULL,
  `ipAddress` VARCHAR(64) NULL,
  `userAgent` VARCHAR(512) NULL,
  UNIQUE INDEX `AuthSession_tokenHash_key`(`tokenHash`),
  INDEX `AuthSession_userId_idx`(`userId`),
  INDEX `AuthSession_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`id`),
  CONSTRAINT `AuthSession_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- パスワードのハッシュ（B1・B2）。値は PHC 文字列（$argon2id$v=19$m=19456,t=2,p=1$塩$ハッシュ）で、
-- アルゴリズムと強さを一緒に持つ。Better Auth の scrypt（塩:ハッシュ の形）も読めて、
-- ログインに成功したときに Argon2id で作り直す。
CREATE TABLE `AuthPassword` (
  `userId` VARCHAR(191) NOT NULL,
  `hash` VARCHAR(255) NOT NULL,
  `updatedAt` DATETIME(3) NOT NULL,
  PRIMARY KEY (`userId`),
  CONSTRAINT `AuthPassword_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 外部ログインの結びつき（F2）。利用者はプロバイダーとプロバイダー側の ID の組で見分け、メールでは見分けない。
-- プロバイダーのアクセストークンは使わないので持たない（F3）。
CREATE TABLE `AuthIdentity` (
  `provider` VARCHAR(32) NOT NULL,
  `providerUserId` VARCHAR(191) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  INDEX `AuthIdentity_userId_idx`(`userId`),
  PRIMARY KEY (`provider`, `providerUserId`),
  CONSTRAINT `AuthIdentity_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- メールで送る1回限りのトークン（E1）。メール確認と再設定。ハッシュで持ち、用途（purpose）に結びつける。
-- 同じ人・同じ用途で新しく発行したら古いものは消す。使ったら消す。
CREATE TABLE `AuthToken` (
  `tokenHash` BINARY(32) NOT NULL,
  `purpose` VARCHAR(32) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  INDEX `AuthToken_userId_purpose_idx`(`userId`, `purpose`),
  INDEX `AuthToken_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`tokenHash`),
  CONSTRAINT `AuthToken_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- パスワードは合ったが、2段階認証がまだの状態（G3）。セッションではなく、別の Cookie と5分の寿命で持つ。
-- ここからできるのはコードの確認だけ。attempts は、この状態のままコードを試した回数（H1）。
CREATE TABLE `AuthMfaChallenge` (
  `tokenHash` BINARY(32) NOT NULL,
  `userId` VARCHAR(191) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  `attempts` INT UNSIGNED NOT NULL DEFAULT 0,
  INDEX `AuthMfaChallenge_userId_idx`(`userId`),
  INDEX `AuthMfaChallenge_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`tokenHash`),
  CONSTRAINT `AuthMfaChallenge_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 認証アプリ（TOTP）の秘密（G1）。AES-256-GCM で暗号化して持ち、鍵は DB の外（環境変数）に置く。
--   secret        「鍵の版:base64(nonce + 暗号文)」。版で鍵を作り直せる（I2）
--   enabledAt     コードを1回確かめて有効にした時刻。NULL は設定の途中
--   lastUsedStep  最後に通したコードの時間ステップ。これ以下のステップは二度と通さない（使い回しの拒否）
CREATE TABLE `AuthTotp` (
  `userId` VARCHAR(191) NOT NULL,
  `secret` VARCHAR(255) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  `enabledAt` DATETIME(3) NULL,
  `lastUsedStep` BIGINT NULL,
  PRIMARY KEY (`userId`),
  CONSTRAINT `AuthTotp_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 予備コード（G2）。80 ビットの乱数なので SHA-256 で持てば総当たりできない（C2 と同じ考え方）。使ったら消す。
CREATE TABLE `AuthBackupCode` (
  `userId` VARCHAR(191) NOT NULL,
  `codeHash` BINARY(32) NOT NULL,
  PRIMARY KEY (`userId`, `codeHash`),
  CONSTRAINT `AuthBackupCode_userId_fkey` FOREIGN KEY (`userId`) REFERENCES `user`(`id`) ON DELETE CASCADE ON UPDATE CASCADE
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 外部ログインの往復の途中の状態（F1・06 C4）。state は Cookie にも置き、両方が合ったときだけ続ける。
-- PKCE の code_verifier と OIDC の nonce は、ここにだけ置く（ブラウザに渡さない）。10分で切れる。
CREATE TABLE `AuthOAuthState` (
  `stateHash` BINARY(32) NOT NULL,
  `provider` VARCHAR(32) NOT NULL,
  `codeVerifier` VARCHAR(128) NOT NULL,
  `nonce` VARCHAR(64) NOT NULL,
  `redirectTo` VARCHAR(512) NOT NULL,
  `createdAt` DATETIME(3) NOT NULL,
  `expiresAt` DATETIME(3) NOT NULL,
  INDEX `AuthOAuthState_expiresAt_idx`(`expiresAt`),
  PRIMARY KEY (`stateHash`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 回数制限（H1・06 B4）。サインインのアカウント単位・IP 単位、2段階認証のアカウント単位などを1つの表で数える。
-- bucket は「種類:対象」の SHA-256（メールアドレスや IP をそのまま残さない）。窓が終わった行は数えるときに少しずつ消す。
-- SignInAttempt（JUK-93）と同じ作り。デプロイの入れ替えでコンテナが2つ並ぶので、数はメモリではなくここに置く。
CREATE TABLE `AuthThrottle` (
  `bucket` BINARY(32) NOT NULL,
  `count` INT UNSIGNED NOT NULL,
  `windowStartedAt` DATETIME(3) NOT NULL,
  INDEX `AuthThrottle_windowStartedAt_idx`(`windowStartedAt`),
  PRIMARY KEY (`bucket`)
) DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- パスワード（Better Auth の account.providerId = 'credential'）を引き継ぐ。形は Better Auth の scrypt のまま入り、
-- 最初のログインで Argon2id に作り直される（B2）。
INSERT IGNORE INTO `AuthPassword` (`userId`, `hash`, `updatedAt`)
SELECT `userId`, `password`, `updatedAt`
FROM `account`
WHERE `providerId` = 'credential' AND `password` IS NOT NULL;

-- Google・GitHub の結びつきを引き継ぐ。Better Auth の accountId は、Google なら sub、GitHub なら数値の ID。
INSERT IGNORE INTO `AuthIdentity` (`provider`, `providerUserId`, `userId`, `createdAt`)
SELECT `providerId`, `accountId`, `userId`, `createdAt`
FROM `account`
WHERE `providerId` IN ('google', 'github');
