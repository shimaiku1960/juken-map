-- 予告なしの実験（JUK-176）の知らせを、API のプロセスが落ちても送れるようにする（JUK-174）。
-- ホストの層の障害（コンテナの kill・メモリの圧迫）では、実験を始めたプロセスが途中で落ちることがあるので、
-- 終わるまで待つのをやめ、起動中のどのプロセスでも「終わったのにまだ知らせていない」行を拾って知らせる。
--
-- startedBy は始めた入口（schedule＝予告なしのくじ、job＝機械の入口 /api/chaos/）。知らせるのは schedule の行だけ。
-- notifiedAt は知らせた時刻。blue/green の切り替えで2つのプロセスが同時に拾っても、先に入れた方だけが送る。
ALTER TABLE `ChaosExperiment`
  ADD COLUMN `startedBy` VARCHAR(32) NOT NULL DEFAULT 'job',
  ADD COLUMN `notifiedAt` DATETIME(3) NULL;
