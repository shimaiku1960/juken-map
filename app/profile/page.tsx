import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import Link from "next/link";
import ProfileEdit from "@/app/components/ProfileEdit";
import { Card, CardContent } from "@/components/ui/card";
import { DEMO_EMAIL } from "@/lib/demo";

const ProfilePage = async () => {
    const session = await auth.api.getSession({
        headers: await headers(),
    });

    if (!session) {
        redirect("/login");
    }

    const user = session.user;
    const nickname = user.nickname ?? user.name ?? "ユーザー";

      return (
        <main className="w-full mx-auto max-w-3xl p-8">
          <h1 className="text-3xl font-bold mb-6">プロフィール</h1>
          <Card>
    <CardContent className="divide-y">
      <div className="flex items-center gap-3 pb-4">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-blue-100 text-lg font-bold text-blue-700">
          {nickname.charAt(0)}
        </div>
        <p className="font-medium">{nickname}</p>
      </div>
      <div className="py-4">
        <p className="text-sm text-gray-500 mb-1">ニックネーム</p>
        <ProfileEdit
          currentNickname={user.nickname ?? ""}
          readOnly={user.email === DEMO_EMAIL}
        />
      </div>
      <div className="pt-4">
        <p className="text-sm text-gray-500 mb-1">メールアドレス</p>
        <p className="text-base">{user.email}</p>
      </div>
    </CardContent>
  </Card>

  <h2 className="text-2xl font-bold mt-10 mb-4">志望校</h2>
  <Card>
    <CardContent className="flex flex-wrap items-center justify-between gap-3 py-5">
      <p className="text-sm text-gray-500">
        志望校の設定・第一志望の切り替えは「志望校」ページに移動しました。
      </p>
      <Link
        href="/goals"
        className="text-sm text-blue-500 hover:underline whitespace-nowrap"
      >
        志望校ページへ →
      </Link>
    </CardContent>
  </Card>
        </main>
      );
    };
  



 
        


export default ProfilePage;
