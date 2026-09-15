// 負荷試験 Phase 0 用のユーザーと、同時完了を試す予定1件を用意する。
// 結果（ログイン情報と予定の id）を JSON で標準出力に書く（scripts/run-loadtest-phase0.sh が読む）。
import {
  execute,
  runSeed,
  setCredentialPassword,
  upsertVerifiedUser,
} from "./seed-helpers";

const EMAIL = "loadtest-phase0@juken-map.invalid";
const PASSWORD = "loadtest-phase0-password";
const PLAN_MARKER = "LOADTEST_PHASE0_CONCURRENT_COMPLETE";

runSeed(async () => {
  // 初回記録の印も消し、試験のたびに「初回の記録」から始められるようにする
  const userId = await upsertVerifiedUser(
    { email: EMAIL, name: "Load Test Phase 0", nickname: "Load Test" },
    { resetFirstStudyLog: true }
  );
  await setCredentialPassword(userId, PASSWORD);

  // 前回の試験で作った予定と、その予定に紐づく実績だけを消す
  await execute(
    `DELETE l FROM StudyLog AS l
     JOIN StudyPlan AS p ON p.id = l.studyPlanId
     WHERE l.userId = ? AND p.content = ?`,
    [userId, PLAN_MARKER]
  );
  await execute("DELETE FROM StudyPlan WHERE userId = ? AND content = ?", [userId, PLAN_MARKER]);
  await execute("DELETE FROM session WHERE userId = ?", [userId]);

  const now = new Date();
  const plan = await execute(
    `INSERT INTO StudyPlan (userId, date, content, subject, done, createdAt, updatedAt)
     VALUES (?, ?, ?, 'english', FALSE, ?, ?)`,
    [userId, now, PLAN_MARKER, now, now]
  );

  process.stdout.write(JSON.stringify({ email: EMAIL, password: PASSWORD, planId: plan.insertId }));
});
