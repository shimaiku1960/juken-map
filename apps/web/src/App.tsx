import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router";
import Header from "@/web/components/Header";
import RequireAuth from "@/web/components/RequireAuth";
import HomePage from "@/web/pages/HomePage";

// トップ（LP）以外のページは開いたときに読み込む。
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
const LoginPage = lazy(() => import("@/web/pages/auth/login"));
const SignUpPage = lazy(() => import("@/web/pages/auth/signup"));
const ForgotPasswordPage = lazy(() => import("@/web/pages/auth/forgot-password"));
const ResetPasswordPage = lazy(() => import("@/web/pages/auth/reset-password"));
const VerifyEmailPage = lazy(() => import("@/web/pages/auth/verify-email"));
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

          {/* LINE のトークから開く。ログインは画面内で判定するので protectedRoute にしない。 */}
          <Route path="/line/link" element={<LineLinkPage />} />

          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </Suspense>
    </div>
  );
}
