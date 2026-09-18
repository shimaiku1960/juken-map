import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router";
import Header from "@/web/components/Header";
import RequireAuth from "@/web/components/RequireAuth";
import HomePage from "@/web/pages/HomePage";
// 認証画面は最初の1本に入れる。LP やメールのリンクから直接開かれることが多く、
// 分けると「最初の JS を実行 → 画面の JS を取りに行く」待ちが1段増える。
// どれも小さい部品だけでできていて、足しても約10KB。
import ForgotPasswordPage from "@/web/pages/auth/forgot-password";
import LoginPage from "@/web/pages/auth/login";
import ResetPasswordPage from "@/web/pages/auth/reset-password";
import SignUpPage from "@/web/pages/auth/signup";
import VerifyEmailPage from "@/web/pages/auth/verify-email";

// それ以外のページは開いたときに読み込む。
// 最初の1本に全ページを入れると、LPだけ見る人にもダッシュボード等を配ってしまう。
const DashboardPage = lazy(() => import("@/web/pages/DashboardPage"));
const GoalsPage = lazy(() => import("@/web/pages/GoalsPage"));
const ExplorePage = lazy(() => import("@/web/pages/ExplorePage"));
const UniversityDetailPage = lazy(() => import("@/web/pages/UniversityDetailPage"));
const ProfilePage = lazy(() => import("@/web/pages/ProfilePage"));
const BlogPage = lazy(() => import("@/web/pages/BlogPage"));
const ArticlePage = lazy(() => import("@/web/pages/ArticlePage"));
const TermsPage = lazy(() => import("@/web/pages/TermsPage"));
const PrivacyPage = lazy(() => import("@/web/pages/PrivacyPage"));
const AdminPage = lazy(() => import("@/web/pages/AdminPage"));
const LineLinkPage = lazy(() => import("@/web/pages/LineLinkPage"));
const NotFoundPage = lazy(() => import("@/web/pages/NotFoundPage"));

const protectedRoute = (element: React.ReactNode) => (
  <RequireAuth>{element}</RequireAuth>
);

export default function App() {
  return (
    <div className="min-h-full flex flex-col">
      <Header />
      {/* ページのチャンク取得中は何も出さない（RequireAuth の判定中と同じ扱い）。 */}
      <Suspense fallback={null}>
        <Routes>
          {/* 公開 */}
          <Route path="/" element={<HomePage />} />
          <Route path="/blog" element={<BlogPage />} />
          <Route path="/articles/:id" element={<ArticlePage />} />
          <Route path="/terms" element={<TermsPage />} />
          <Route path="/privacy" element={<PrivacyPage />} />
          {/* LP はトップに統合済み。旧 URL は `/` へ寄せる（Next.js 版は 308 リダイレクト）。 */}
          <Route path="/lp" element={<Navigate to="/" replace />} />

          {/* 認証フロー */}
          <Route path="/login" element={<LoginPage />} />
          <Route path="/signup" element={<SignUpPage />} />
          <Route path="/forgot-password" element={<ForgotPasswordPage />} />
          <Route path="/reset-password" element={<ResetPasswordPage />} />
          <Route path="/verify-email" element={<VerifyEmailPage />} />

          {/* ログイン必須 */}
          <Route path="/dashboard" element={protectedRoute(<DashboardPage />)} />
          <Route path="/goals" element={protectedRoute(<GoalsPage />)} />
          <Route path="/explore" element={protectedRoute(<ExplorePage />)} />
          <Route
            path="/explore/:universityId"
            element={protectedRoute(<UniversityDetailPage />)}
          />
          <Route path="/profile" element={protectedRoute(<ProfilePage />)} />
          {/* Next.js 版と同じく /schedule はダッシュボードのカレンダーへ寄せる */}
          <Route
            path="/schedule"
            element={<Navigate to="/dashboard#study-calendar" replace />}
          />

          {/* 管理者だけ（権限は API が判定し、それ以外には「権限がありません」を出す） */}
          <Route path="/admin" element={protectedRoute(<AdminPage />)} />

          {/* LINE のトークから開く。ログインは画面内で判定するので protectedRoute にしない。 */}
          <Route path="/line/link" element={<LineLinkPage />} />

          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </Suspense>
    </div>
  );
}
