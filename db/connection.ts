import mysql, {
  type Pool,
  type PoolConnection,
  type ResultSetHeader,
} from "mysql2/promise";

// Node から DB に繋ぐときの接続プール。seed（db/seed-helpers.ts）と、本物の DB に流す
// テスト（test-db/）が使う。アプリの接続は Go 側（apps/api-go/db.go）にあり、時間帯と真偽値の
// 扱いはそちらと揃える（Go が書いた行を seed やテストが読み、その逆もあるため）。

const pool: Pool = mysql.createPool({
  ...parseDatabaseUrl(process.env.DATABASE_URL),
  // 同時に張るDB接続の本数の上限。超えた分はプールの中で順番待ちになる。
  connectionLimit: Number(process.env.DB_POOL_LIMIT ?? 15),
  // MySQL の DATETIME は時間帯を持たない「ただの日時」で、どの時間帯として
  // 読み書きするかはクライアントが決める。既定はプロセスのローカル時刻なので、
  // Mac（JST）で動かすと、UTC として保存している既存の値と9時間ずれる。
  timezone: "Z",
  typeCast(field, next) {
    // MySQL に真偽値型は無く、BOOLEAN は TINYINT(1) の別名。そのままだと 1 / 0 が
    // 返ってくるので、true / false に直す。
    if (field.type === "TINY" && field.length === 1) {
      const value = field.string();
      return value === null ? null : value === "1";
    }
    return next();
  },
});

/** DATABASE_URL を mysql2 の接続設定にする。 */
export function parseDatabaseUrl(url: string | undefined) {
  // 読み込むだけで DB を使わない場面もあるので、URL が無くても読み込み時には落とさない。
  // 実際に繋ぐときに接続エラーになる。
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

async function run(db: Db, sql: string, params: unknown[]) {
  // 値は必ず params で渡す。SQL 文字列に埋め込むと SQL インジェクションになる。
  // execute（プリペアドステートメント）ではなく query を使うのは、
  // `IN (?)` に配列を渡して展開させたいから。エスケープはドライバが行う。
  const [result] = await db.query(sql, params);
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

export { pool };
