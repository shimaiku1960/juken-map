/**
 * 公開サイトの正規 URL。
 *
 * metadataBase・robots.txt・sitemap.xml が同じ値を参照できるよう 1 か所にまとめる。
 * apex（www 無し）が正で、www は apex へ寄せる方針（[[aws-ec2-deploy]] 参照）。
 */
export const SITE_URL = "https://juken-map.com";

/** LINE公式アカウントの友だち追加・トーク画面。Bot basic IDは公開情報。 */
export const LINE_OFFICIAL_ACCOUNT_URL = "https://line.me/R/ti/p/@629pxqus";

/**
 * 検索結果に載せないページ用の robots 設定。
 *
 * ルートレイアウトの noindex を外したあとも、ログイン後ページと認証フローの
 * ページは検索に出したくないので、各ページで個別に上書きする。
 */
export const NOINDEX = { index: false, follow: false } as const;
