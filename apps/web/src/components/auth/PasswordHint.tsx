// 新しいパスワードを決める欄の下に出す規則の案内（認証基準 10 の A2、apps/api の checkNewPassword）。
// 長さで強さを持たせ、記号や大文字は求めない。よく使われているパスワードはサーバーが断る。
export default function PasswordHint({ id }: { id: string }) {
  return (
    <p id={id} className="text-xs leading-5 text-muted-foreground">
      15文字以上。記号や大文字は要りません。覚えやすい長い文（例：「毎朝6時に英単語を30個」をローマ字で）がおすすめです。
      パスワード管理アプリの自動生成も使えます。
    </p>
  );
}
