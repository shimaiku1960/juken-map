-- user の sim の列から UserSimulation を埋める（JUK-228）。docs/migrations.md の決まり1（足す → 移す → 消す）の「移す」。
--
-- 前のデプロイ（20261112090000_add_user_simulation_table）から、アプリは書き込みのたびにその利用者の行を
-- UserSimulation へ写している。ここでは、それより前に印が付いたまま書き込みの無かった利用者を埋める。
-- このマイグレーションが流れる間に動いているのは前のアプリ（両方に書く）なので、ここで埋めた後にできた印も写される。
--
-- まだ UserSimulation に行が無い利用者だけを入れるので、何度流しても壊れない。
-- 行数は合成ユーザーの数（多くても数千）なので、1本の INSERT で流す。
-- このデプロイから、アプリは UserSimulation だけを読み書きする。user の sim の列は次のデプロイで消す。
INSERT INTO UserSimulation (userId, seq, cohort, dormantFrom, lastActedOn)
SELECT u.id, u.simSeq, u.simCohort, u.simDormantFrom, u.simLastActedOn
FROM `user` u
LEFT JOIN UserSimulation s ON s.userId = u.id
WHERE u.simSeq IS NOT NULL AND u.simCohort IS NOT NULL AND s.userId IS NULL;
