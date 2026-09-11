import { execute, select } from "@/api/infra/db";
import type {
  TextbookMasterMetricRow,
  TextbookMasterRow,
  TextbookRow,
} from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";

const TEXTBOOK_COLUMNS = `
  id, userId, masterId, name, totalAmount, rangeUnit, targetDate, subject, createdAt, updatedAt
`;

type MasterWithMetricRow = TextbookMasterRow & {
  m_id: number | null;
  m_unit: string | null;
  m_totalAmount: number | null;
  m_isDefault: boolean | null;
  m_createdAt: Date | null;
  m_updatedAt: Date | null;
};

// マスター → 総量の候補は一本道の1対多なので、LEFT JOIN 1本で取り、マスターごとに束ねる。
// 総量の候補は id 順に並べる。登録時は isDefault の候補、無ければ先頭の候補を使うので、
// 先頭が何かを決めておく必要がある（Prisma の findTextbookMaster は並びを指定していなかった）。
const MASTER_SQL = `
  SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn, tm.createdAt, tm.updatedAt,
         m.id AS m_id, m.unit AS m_unit, m.totalAmount AS m_totalAmount,
         m.isDefault AS m_isDefault, m.createdAt AS m_createdAt, m.updatedAt AS m_updatedAt
  FROM TextbookMaster AS tm
  LEFT JOIN TextbookMasterMetric AS m ON m.masterId = tm.id
`;

function groupMasters(rows: MasterWithMetricRow[]) {
  const masters: (TextbookMasterRow & { metrics: TextbookMasterMetricRow[] })[] = [];
  for (const row of rows) {
    let master = masters.at(-1);
    if (master?.id !== row.id) {
      master = {
        id: row.id,
        name: row.name,
        publisher: row.publisher,
        edition: row.edition,
        isbn: row.isbn,
        createdAt: row.createdAt,
        updatedAt: row.updatedAt,
        metrics: [],
      };
      masters.push(master);
    }
    if (row.m_id !== null) {
      master.metrics.push({
        id: row.m_id,
        masterId: row.id,
        unit: row.m_unit!,
        totalAmount: row.m_totalAmount!,
        isDefault: row.m_isDefault!,
        createdAt: row.m_createdAt!,
        updatedAt: row.m_updatedAt!,
      });
    }
  }
  return masters;
}

export function listTextbookMasters() {
  return measured("textbookMaster.list", async () =>
    groupMasters(await select<MasterWithMetricRow>(`${MASTER_SQL} ORDER BY tm.id ASC, m.id ASC`))
  );
}

/**
 * 指定した参考書のうち、自分が所有しているものの件数を返す。
 * 呼び出し元は「渡した件数と一致するか」で他人の参考書混入を弾く。
 */
export function countOwnedTextbooks(ids: number[], userId: string) {
  return measured("textbook.countOwned", async () => {
    // IN () は空だと SQL の構文エラーになる。空なら数えるまでもなく0件。
    if (ids.length === 0) return 0;
    const [row] = await select<{ count: number }>(
      "SELECT COUNT(*) AS count FROM Textbook WHERE id IN (?) AND userId = ?",
      [ids, userId]
    );
    return Number(row.count);
  });
}

/** 1件だけの所有確認。範囲や単位の検証にも使うので行ごと返す。 */
export function findOwnedTextbook(id: number, userId: string) {
  return measured("textbook.findOwned", async () => {
    const [row] = await select<TextbookRow>(
      `SELECT ${TEXTBOOK_COLUMNS} FROM Textbook WHERE id = ? AND userId = ? LIMIT 1`,
      [id, userId]
    );
    return row ?? null;
  });
}

/** 自分の参考書の一覧。 */
export function listTextbooks(userId: string) {
  return measured("textbook.list", () =>
    // 名前は (userId, name) で UNIQUE なので、名前順だけで並びが決まる。
    select<TextbookRow>(
      `SELECT ${TEXTBOOK_COLUMNS} FROM Textbook WHERE userId = ? ORDER BY name ASC`,
      [userId]
    )
  );
}

/** マスター登録から作るときの元データ。総量の候補（metrics）も一緒に引く。 */
export function findTextbookMaster(id: number) {
  return measured("textbookMaster.find", async () => {
    const [master] = groupMasters(
      await select<MasterWithMetricRow>(`${MASTER_SQL} WHERE tm.id = ? ORDER BY m.id ASC`, [id])
    );
    return master ?? null;
  });
}

async function findTextbookById(id: number) {
  const [row] = await select<TextbookRow>(
    `SELECT ${TEXTBOOK_COLUMNS} FROM Textbook WHERE id = ?`,
    [id]
  );
  if (!row) throw new Error(`Textbook ${id} が見つかりません`);
  return row;
}

/**
 * 参考書を登録する。同名の重複は DB の一意制約（userId, name）が弾き、
 * ER_DUP_ENTRY の例外になる。ルートがそれを 409 に翻訳する。
 */
export function createTextbook(data: {
  name: string;
  userId: string;
  masterId?: number;
  totalAmount?: number;
  rangeUnit?: string;
  subject?: string | null;
}) {
  return measured("textbook.create", async () => {
    const now = new Date();
    const inserted = await execute(
      `INSERT INTO Textbook
         (userId, name, masterId, totalAmount, rangeUnit, subject, createdAt, updatedAt)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        data.userId,
        data.name,
        data.masterId ?? null,
        data.totalAmount ?? null,
        data.rangeUnit ?? null,
        data.subject ?? null,
        now,
        now,
      ]
    );
    return findTextbookById(inserted.insertId);
  });
}

/** 逆算設定を更新する。送られてきた項目だけ変える。 */
export function updateTextbookProgress(
  id: number,
  data: {
    totalAmount?: number;
    rangeUnit?: string;
    targetDate?: string | null;
    subject?: string | null;
  }
) {
  return measured("textbook.updateProgress", async () => {
    // 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す。
    const changes: [column: string, value: unknown][] = [];
    if (data.totalAmount !== undefined) changes.push(["totalAmount", data.totalAmount]);
    if (data.rangeUnit !== undefined) changes.push(["rangeUnit", data.rangeUnit]);
    if (data.targetDate !== undefined) {
      // 目標日は日付だけを持つので、UTC の 0 時として保存する。
      changes.push([
        "targetDate",
        data.targetDate == null ? null : new Date(`${data.targetDate}T00:00:00.000Z`),
      ]);
    }
    if (data.subject !== undefined) changes.push(["subject", data.subject]);
    changes.push(["updatedAt", new Date()]);

    await execute(
      `UPDATE Textbook SET ${changes.map(([column]) => `${column} = ?`).join(", ")}
       WHERE id = ?`,
      [...changes.map(([, value]) => value), id]
    );
    return findTextbookById(id);
  });
}
