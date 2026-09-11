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
  connectionLimit: 5,
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

function parseDatabaseUrl(url: string | undefined) {
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

async function run(db: Db, sql: string, params: unknown[]) {
  const startedAt = performance.now();
  // 値は必ず params で渡す。SQL 文字列に埋め込むと SQL インジェクションになる。
  // execute（プリペアドステートメント）ではなく query を使うのは、
  // `IN (?)` に配列を渡して展開させたいから。エスケープはドライバが行う。
  const [result] = await db.query(sql, params);
  if (shouldLogQueries()) {
    const ms = (performance.now() - startedAt).toFixed(1);
    console.log(`[sql ${ms}ms] ${sql.replace(/\s+/g, " ").trim()}`, params);
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
