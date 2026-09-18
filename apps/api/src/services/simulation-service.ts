import { execute, select } from "@/api/infra/db";
import { measured } from "@/api/observability/measured";
import { SIM_EMAIL_LIKE } from "@/shared/synthetic";

// シミュレーション（sim/）の管理情報を読み書きする。
//
// どの SQL もシミュレーション用のメールアドレスの形を WHERE に入れている。simSeq が付いているか
// だけで絞ると、何かの間違いで実ユーザーに simSeq が付いたときに触ってしまう。二重に絞る。

export type SimUserState = {
  seq: number;
  email: string;
  cohort: string;
  /** 登録した日時（ISO）。実際の動線と同じく createdAt。日付は Asia/Tokyo でシミュレータが数える。 */
  createdAt: string;
  dormantFrom: string | null;
  lastActedOn: string | null;
};

type SimUserRow = {
  simSeq: number;
  email: string;
  simCohort: string;
  createdAt: Date;
  dormantFrom: string | null;
  lastActedOn: string | null;
};

/** 合成ユーザーの一覧と、次に使う連番。 */
export function getSimulationState() {
  return measured("simulation.state", async () => {
    // DATE 列は Date にするとタイムゾーンの解釈が挟まるので、文字列のまま返す。
    const rows = await select<SimUserRow>(
      `SELECT simSeq, email, simCohort, createdAt,
              DATE_FORMAT(simDormantFrom, '%Y-%m-%d') AS dormantFrom,
              DATE_FORMAT(simLastActedOn, '%Y-%m-%d') AS lastActedOn
       FROM \`user\`
       WHERE simSeq IS NOT NULL AND email LIKE ?
       ORDER BY simSeq ASC`,
      [SIM_EMAIL_LIKE]
    );
    const users: SimUserState[] = rows.map((row) => ({
      seq: row.simSeq,
      email: row.email,
      cohort: row.simCohort,
      createdAt: row.createdAt.toISOString(),
      dormantFrom: row.dormantFrom,
      lastActedOn: row.lastActedOn,
    }));
    const nextSeq = users.length > 0 ? users[users.length - 1].seq + 1 : 1;
    return { nextSeq, users };
  });
}

/**
 * 登録を済ませた合成ユーザーに連番と続き方の型を付ける。
 * 付けられたら true。該当するユーザーがいなければ false。
 */
export function markSimulationUser(
  email: string,
  data: { seq: number; cohort: string }
) {
  return measured("simulation.markUser", async () => {
    const result = await execute(
      `UPDATE \`user\` SET simSeq = ?, simCohort = ?, updatedAt = ?
       WHERE email = ? AND email LIKE ?`,
      [data.seq, data.cohort, new Date(), email, SIM_EMAIL_LIKE]
    );
    return result.affectedRows > 0;
  });
}

/** 最後に操作した日・来なくなった日を記録する。undefined の項目は変えない。 */
export function updateSimulationUser(
  seq: number,
  data: { lastActedOn?: string | null; dormantFrom?: string | null }
) {
  return measured("simulation.updateUser", async () => {
    const sets: string[] = [];
    const params: unknown[] = [];
    if (data.lastActedOn !== undefined) {
      sets.push("simLastActedOn = ?");
      params.push(data.lastActedOn);
    }
    if (data.dormantFrom !== undefined) {
      sets.push("simDormantFrom = ?");
      params.push(data.dormantFrom);
    }
    if (sets.length === 0) return true;
    const result = await execute(
      `UPDATE \`user\` SET ${sets.join(", ")}
       WHERE simSeq = ? AND email LIKE ?`,
      [...params, seq, SIM_EMAIL_LIKE]
    );
    return result.affectedRows > 0;
  });
}
