-- 管理者ページ（/admin）からの利用者の停止。NULL なら停止していない。
-- 停止すると：その人の session を全部消す（すぐログアウト）／Better Auth の
-- session.create.before（apps/api/src/auth.ts）が新しいログインを断る。
-- 列を消さずに日時で持つのは、いつ止めたかを一覧に出し、解除を NULL に戻すだけで済ませるため。
ALTER TABLE `user`
  ADD COLUMN `bannedAt` DATETIME(3) NULL;
