-- 管理者ページ（/admin）の権限。値は 'user' / 'admin' の2つだけ（型はアプリ側で縛る）。
-- 既存ユーザーは全員 'user'。admin は画面からは付けられず、pnpm admin:grant で付ける。
-- Better Auth の additionalFields では input: false にしてあり、利用者が自分で書き換えることはできない。
ALTER TABLE `user`
  ADD COLUMN `role` VARCHAR(32) NOT NULL DEFAULT 'user';
