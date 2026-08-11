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

/** 契約者が利用するLINE公式アカウントの友だち追加URL。 */
export const SUPPORT_LINE_ADD_URL = "https://lin.ee/5ayqEg0";

/** 連携済み契約者がLINE公式アカウントのトークを開くURL。 */
export const SUPPORT_LINE_CHAT_URL =
  "https://line.me/R/oaMessage/%40100ncdqf";

/** 面談申し込みメールの件名（メール申し込みへ切り替わったときに使う）。 */
export const SUPPORT_MAIL_SUBJECT = "受験英語サポート 無料面談の申し込み";

/** 無料体験終了後に毎月請求する価格（税込）。 */
export const SUPPORT_MONTHLY_PRICE_TAX_INCLUDED = 1_980;

/** 無料体験の提供時間（7 日 × 24 時間）。 */
export const SUPPORT_TRIAL_HOURS = 168;

/** 画面表示用の無料体験日数。 */
export const SUPPORT_TRIAL_DAYS = SUPPORT_TRIAL_HOURS / 24;

/** 同時にサポートする人数の上限。達している間は無料面談の受付を止める。 */
export const SUPPORT_CAPACITY = 5;

/** 最終確認画面で示す1年間の支払額目安（無料体験後に12回更新した場合）。 */
export const SUPPORT_ANNUAL_PRICE_ESTIMATE =
  SUPPORT_MONTHLY_PRICE_TAX_INCLUDED * 12;

/** サービス提供者の公開上の名称。正式氏名等は請求窓口から遅滞なく開示する。 */
export const SUPPORT_OPERATOR_NAME = "受験マップ運営（早稲田の高橋）";

/** メール申し込み用の mailto リンク。 */
export const supportMailtoHref = `mailto:${SUPPORT_CONTACT_EMAIL}?subject=${encodeURIComponent(
  SUPPORT_MAIL_SUBJECT
)}`;
