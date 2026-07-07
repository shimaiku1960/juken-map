import Link from "next/link";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import { logout } from "@/app/auth/actions";

const Header = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  const user = session?.user;
                                                                                                     
  return (                                                                                         
    <header className="border-b px-8 py-4 flex justify-between items-center">
      <Link href="/" className="text-xl font-bold">
        受験マップ
      </Link>                                                                                        
      <nav className="flex gap-4 items-center">
        <Link href="/blog" className="text-blue-500 hover:underline">
          ブログ
        </Link>
        {user ? (
          <>
            <Link href="/explore" className="text-blue-500 hover:underline">
              大学を探す
            </Link>
            <Link href="/schedule" className="text-blue-500 hover:underline">
              学習予定
            </Link>
            <Link href="/profile" className="text-blue-500 hover:underline">
              プロフィール
            </Link>                
            <form action={logout}>                                                                   
              <button type="submit" className="text-red-500 hover:underline">                        
                ログアウト
              </button>                                                                              
            </form>                                                                                
          </>
        ) : (                                                                                        
          <Link href="/login" className="text-blue-500 hover:underline">
            ログイン                                                                                 
          </Link>                                                                                  
        )}
      </nav>
    </header>
  );                                        
};                                      

export default Header; 