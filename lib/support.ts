/**
 * 受験英語 LINE 質問サポートの提供条件。
 *
 * 公開ページ（/support）と sitemap が同じ値を参照できるよう 1 か所にまとめる。
 * SITE_URL と同じ方針で、環境変数ではなくコード上の定数として持つ。
 */

/**
 * 無料 30 分オンライン面談の予約ページ URL（Google カレンダーの予約スケジュール）。
 *
 * サービス専用アカウント「受験マップ運営」で作成した予約ページ。
 * 毎週火・木 19:00〜21:00 の 30 分枠、48 時間前まで、14 日先まで、1 日 2 件。
 * 予約フォームで年齢区分（18 歳以上／未満）と相談内容を必須で受け取る。
 *
 * この値が `null` のあいだ、/support は未公開モードになる（CTA がメール申し込みへ
 * 切り替わり、noindex ＋ sitemap から除外され、公開ナビゲーションにも出さない）。
 */
export const SUPPORT_BOOKING_URL: string | null =
  null;

/** 申し込み・問い合わせ先。利用規約の運営者連絡先と揃える。 */
export const SUPPORT_CONTACT_EMAIL = "waseda.no.takahashi@gmail.com";

/** 面談申し込みメールの件名（メール申し込みへ切り替わったときに使う）。 */
export const SUPPORT_MAIL_SUBJECT = "受験英語サポート 無料面談の申し込み";

/** 30 日間の買い切り価格（税込）。自動更新はしない。 */
export const SUPPORT_PRICE_TAX_INCLUDED = 3_980;

/** 決済日を 1 日目として数える提供期間（日）。 */
export const SUPPORT_PERIOD_DAYS = 30;

/** 同時にサポートする人数の上限。達している間は無料面談の受付を止める。 */
export const SUPPORT_CAPACITY = 5;

/** メール申し込み用の mailto リンク。 */
export const supportMailtoHref = `mailto:${SUPPORT_CONTACT_EMAIL}?subject=${encodeURIComponent(
  SUPPORT_MAIL_SUBJECT
)}`;
