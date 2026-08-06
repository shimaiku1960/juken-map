import type { Metadata } from "next";
import LegalDocument from "@/app/components/legal/LegalDocument";
import { NOINDEX } from "@/lib/site";
import {
  SUPPORT_ANNUAL_PRICE_ESTIMATE,
  SUPPORT_CAPACITY,
  SUPPORT_CONTACT_EMAIL,
  SUPPORT_MONTHLY_PRICE_TAX_INCLUDED,
  SUPPORT_OPERATOR_NAME,
  SUPPORT_TRIAL_HOURS,
} from "@/lib/support";

export const metadata: Metadata = {
  title: "特定商取引法に基づく表記｜受験マップ",
  description: "受験英語LINE質問サポートの取引条件を表示します。",
  robots: NOINDEX,
};

const price = SUPPORT_MONTHLY_PRICE_TAX_INCLUDED.toLocaleString("ja-JP");
const annualPrice = SUPPORT_ANNUAL_PRICE_ESTIMATE.toLocaleString("ja-JP");

export default function CommercialTransactionsPage() {
  return (
    <LegalDocument
      title="特定商取引法に基づく表記"
      description="受験英語LINE質問サポートのお申し込み前に、料金・提供条件・解約条件をご確認ください。"
      effectiveDate="2026年8月6日"
      sections={[
        {
          title: "役務提供事業者",
          content: (
            <>
              <p>{SUPPORT_OPERATOR_NAME}</p>
              <p>
                正式氏名、住所および電話番号は、消費者からの請求があった場合、申込みの判断前に遅滞なく開示します。開示をご希望の方は
                <a href={`mailto:${SUPPORT_CONTACT_EMAIL}`}>{SUPPORT_CONTACT_EMAIL}</a>
                へご連絡ください。
              </p>
            </>
          ),
        },
        {
          title: "役務の内容",
          content: (
            <p>
              LINE公式アカウントを通じて、大学受験英語の勉強法、教材、個別問題、過去問、学習計画などの質問に回答します。質問回数に数値上の上限はありませんが、大量の全問解説、継続的な大量添削、英語以外の詳細指導、宿題の代行などは対象外です。
            </p>
          ),
        },
        {
          title: "販売価格",
          content: (
            <>
              <p>月額{price}円（税込）です。</p>
              <p>
                初回は契約成立時から{SUPPORT_TRIAL_HOURS}時間無料です。無料体験終了後は、解約されるまで月単位で自動更新されます。12回の月額料金が発生した場合の支払額目安は{annualPrice}円（税込）です。
              </p>
            </>
          ),
        },
        {
          title: "販売価格以外の負担",
          content: (
            <p>
              サービスの利用に必要なインターネット接続料金、通信料金、端末費用などは利用者の負担です。これらの金額は利用者が契約する事業者の条件によります。
            </p>
          ),
        },
        {
          title: "支払方法と支払時期",
          content: (
            <p>
              クレジットカードでお支払いいただく予定です。無料体験終了日時に初回の月額料金を請求し、以後は決済事業者が定める同じ基準日時に月額料金を請求します。実際の無料体験終了日時、初回請求日時、対応する支払方法は申込最終確認画面に表示します。
            </p>
          ),
        },
        {
          title: "役務の提供時期と期間",
          content: (
            <p>
              決済情報の登録と申込みの完了後に無料体験を開始します。無料体験および有料期間中、LINE公式アカウントの連携完了後から質問できます。解約されるまで月単位で更新されます。
            </p>
          ),
        },
        {
          title: "申込条件と定員",
          content: (
            <p>
              原則として無料オンライン面談後、運営者から案内を受けた方がお申し込みいただけます。無料体験中と有料契約中を合わせて同時{SUPPORT_CAPACITY}人を上限とし、満員時は新規受付を停止します。18歳未満の方は、法定代理人の事前同意を確認した後にのみ申し込めます。
            </p>
          ),
        },
        {
          title: "解約と返金",
          content: (
            <ul>
              <li>いつでも次回更新を停止できます。</li>
              <li>無料体験中に解約した場合、月額料金は請求しません。</li>
              <li>有料期間中の解約後も、支払済み期間の終了まで利用できます。</li>
              <li>利用者都合による支払済み料金の日割り返金は行いません。</li>
              <li>運営者都合で事前に休止する場合や、予告なく連続72時間以上提供できない場合は、条件に応じて利用期間を延長します。再開できない場合は、支払済み期間の残存期間に応じて返金します。</li>
            </ul>
          ),
        },
        {
          title: "動作環境と注意事項",
          content: (
            <p>
              LINE、Google Meetおよび対応ブラウザを利用できる環境が必要です。返信はできる限り24時間以内を目安としますが、返信時間、合格、成績向上その他の成果を保証するものではありません。
            </p>
          ),
        },
        {
          title: "お問い合わせ",
          content: (
            <p>
              取引条件、解約または事業者情報の開示については、
              <a href={`mailto:${SUPPORT_CONTACT_EMAIL}`}>{SUPPORT_CONTACT_EMAIL}</a>
              へご連絡ください。
            </p>
          ),
        },
      ]}
    />
  );
}
