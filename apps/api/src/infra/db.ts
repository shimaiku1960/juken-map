import mysql, {
  type Pool,
  type PoolConnection,
  type ResultSetHeader,
} from "mysql2/promise";

// アプリ唯一の DB 接続プール。services の生 SQL も、Better Auth（auth.ts）も
// このプールを使う。
//
// Prisma を使っていた頃は、@prisma/adapter-mariadb がこの層を持ち、その上で
// 「クエリの組み立て」と「結果の整形」を ORM がやっていた。ORM を外すとは、
// その2つを services に自分で書くということ。接続の管理はどちらでも要る。

const pool: Pool = mysql.createPool({
  ...parseDatabaseUrl(process.env.DATABASE_URL),
  // 同時に張るDB接続の本数。ここが実質の同時処理数の上限で、超えた分はアプリ側で
  // 順番待ちになる（DBが混む前にここで詰まる）。
  //
  // 2026-09-23に 5 から 15 へ上げた。ダッシュボードの集約API（/api/dashboard）が
  // 実績・予定・日別合計の3本を同時に投げるので、1リクエストが5本中3本を占める。
  // 5のままだと限界点が 500→300 RPS へ落ちることを実測で確認した（15なら500で
  // p95 12.1ms）。⚠️2026-09-21に「プールは主因ではない」と測ったのは、当時の
  // 詰まり所が応答の大きさだったため。詰まり所が変わったので結論も変わった。
  connectionLimit: Number(process.env.DB_POOL_LIMIT ?? 15),
  // MySQL の DATETIME は時間帯を持たない「ただの日時」で、どの時間帯として
  // 読み書きするかはクライアントが決める。既定はプロセスのローカル時刻なので、
  // Mac（JST）で動かすと Prisma が保存してきた値と9時間ずれる。
  // Prisma は UTC として扱っていたので、ここでも UTC に揃える。
  timezone: "Z",
  typeCast(field, next) {
    // MySQL に真偽値型は無く、BOOLEAN は TINYINT(1) の別名。そのままだと 1 / 0 が
    // 返ってくる。Prisma はスキーマの Boolean を見て true / false に直していた。
    if (field.type === "TINY" && field.length === 1) {
      const value = field.string();
      return value === null ? null : value === "1";
    }
    return next();
  },
});

/** DATABASE_URL を mysql2 の接続設定にする。マイグレーション（infra/migrations.ts）も使う。 */
export function parseDatabaseUrl(url: string | undefined) {
  // テストのようにサービスを読み込むだけで DB を使わない場面もあるので、
  // URL が無くても読み込み時には落とさない。実際に繋ぐときに接続エラーになる。
  if (!url) return {};
  const parsed = new URL(url);
  // allowPublicKeyRetrieval などの接続文字列のオプションは mariadb 向けのもの。
  // mysql2 は caching_sha2_password の公開鍵を必要なときに自分で取りに行くので不要。
  return {
    host: parsed.hostname,
    port: Number(parsed.port || 3306),
    user: decodeURIComponent(parsed.username),
    password: decodeURIComponent(parsed.password),
    database: parsed.pathname.slice(1),
    // mysql2 は指定しない限り平文で繋ぐ。本番の RDS へは暗号化して繋ぐ。
    // "Amazon RDS" は mysql2 が同梱する RDS の CA 証明書で、サーバー証明書を検証する。
    // ローカルと CI の MySQL は TLS の証明書を持たないので対象外。
    ...(parsed.hostname.endsWith(".rds.amazonaws.com") && { ssl: "Amazon RDS" }),
  };
}

/** プールそのもの、またはトランザクション中の接続。どちらにも同じ SQL を流せる。 */
export type Db = Pool | PoolConnection;

// Prisma の `log: ["query"]` の代わり。何が DB に飛んでいるかを開発中に見えるようにする。
// SQL_LOG=off で止められる（seed は標準出力を JSON などに使うので止めている）。
// 読み込み時ではなく毎回判定するのは、読み込んだ側があとから止められるようにするため。
function shouldLogQueries() {
  const env = process.env.NODE_ENV;
  return env !== "production" && env !== "test" && process.env.SQL_LOG !== "off";
}

export type QueryLogEntry = {
  sql: string;
  params: unknown[];
  duration_ms: number;
};

let onQuery: ((entry: QueryLogEntry) => void) | undefined;

/**
 * SQL 1本ごとの書き出し先を差し込む。API は pino のロガーを繋ぐ（server.ts）。
 *
 * ここで logger を直接 import しないのは、この層を API 以外からも使うため。
 * seed（db/seed-helpers.ts）とマイグレーションは apps/api の外から読み込むので、
 * observability を巻き込むと fastify や pino まで道連れになる。
 * 差し込まれなければ何も書かないので、seed は今までどおり静かなまま。
 */
export function setQueryLogger(fn: ((entry: QueryLogEntry) => void) | undefined) {
  onQuery = fn;
}

async function run(
  db: Db,
  sql: string,
  params: unknown[],
  options: { dateStrings?: boolean } = {}
) {
  const startedAt = performance.now();
  // 値は必ず params で渡す。SQL 文字列に埋め込むと SQL インジェクションになる。
  // execute（プリペアドステートメント）ではなく query を使うのは、
  // `IN (?)` に配列を渡して展開させたいから。エスケープはドライバが行う。
  const [result] = await db.query({ sql, ...options }, params);
  if (onQuery && shouldLogQueries()) {
    onQuery({
      sql: sql.replace(/\s+/g, " ").trim(),
      params,
      duration_ms: Number((performance.now() - startedAt).toFixed(1)),
    });
  }
  return result;
}

/** SELECT を流し、行の配列を返す。行の型は呼び出し側が宣言する。 */
export async function select<T>(
  sql: string,
  params: unknown[] = [],
  db: Db = pool
): Promise<T[]> {
  return (await run(db, sql, params)) as T[];
}

/** Date の列を、DB が返す "YYYY-MM-DD HH:MM:SS.mmm" の文字列に置き換えた行の型。 */
export type DateStrings<T> = {
  [K in keyof T]: T[K] extends Date
    ? string
    : T[K] extends Date | null
      ? string | null
      : T[K];
};

/**
 * select と同じだが、DATETIME の列を Date にせず、DB が返した文字列のまま受け取る。
 * 画面へ返す一覧のように、Date にしてすぐ ISO 文字列へ戻すだけの場面で使い、
 * 文字列は toIsoString で ISO の形へ直す。
 *
 * mysql2 が文字列を Date にし、それを toISOString で文字列へ戻す往復は1値あたり約750ns、
 * 文字列の組み替えなら約20ns。ダッシュボードでは1リクエストの CPU の約4分の1が
 * この往復だった（2026-09-25 のプロファイル、JUK-49）。
 *
 * プール全体の設定（dateStrings）にしないのは、Better Auth が同じプールで
 * セッションの有効期限などを Date として受け取る前提だから。
 */
export async function selectDateStrings<T>(
  sql: string,
  params: unknown[] = [],
  db: Db = pool
): Promise<DateStrings<T>[]> {
  return (await run(db, sql, params, { dateStrings: true })) as DateStrings<T>[];
}

/**
 * selectDateStrings で受け取った DATETIME(3) の文字列を、Date#toISOString と同じ形にする。
 * DB の値は UTC として保存している（上の timezone: "Z"）ので、末尾に Z を付けるだけでよい。
 * 小数3桁が付く前提なので、DATETIME(3) 以外の列には使わない。
 */
export function toIsoString(value: string): string {
  return `${value.slice(0, 10)}T${value.slice(11)}Z`;
}

/**
 * INSERT / UPDATE / DELETE を流す。
 * 何行変わったか（affectedRows）と、採番された ID（insertId）が返る。
 */
export async function execute(
  sql: string,
  params: unknown[] = [],
  db: Db = pool
): Promise<ResultSetHeader> {
  return (await run(db, sql, params)) as ResultSetHeader;
}

/**
 * fn の中の SQL をすべて1つのトランザクションで流す。
 * fn が例外を投げたら全部取り消し、最後まで進んだら確定する。
 *
 * Prisma の `$transaction(async (tx) => ...)` がやっていたのと同じこと。
 * プールから接続を1本借り、その接続だけで BEGIN から COMMIT までを流す。
 * 別の接続で流した SQL はトランザクションの外になるので、fn の中では
 * 必ず受け取った tx を渡すこと。
 */
export async function transaction<T>(
  fn: (tx: PoolConnection) => Promise<T>
): Promise<T> {
  const connection = await pool.getConnection();
  try {
    await connection.beginTransaction();
    const result = await fn(connection);
    await connection.commit();
    return result;
  } catch (error) {
    await connection.rollback();
    throw error;
  } finally {
    // 借りた接続は必ず返す。返し忘れるとプールが枯れ、全リクエストが止まる。
    connection.release();
  }
}

/**
 * 一意制約違反か。Prisma の P2002 にあたる MySQL のエラー（1062 ER_DUP_ENTRY）。
 */
export function isDuplicateEntry(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    error.code === "ER_DUP_ENTRY"
  );
}

export { pool };
