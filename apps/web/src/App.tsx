import { Navigate, Route, Routes } from "react-router";
import Header from "@/web/components/Header";
import RequireAuth from "@/web/components/RequireAuth";
import HomePage from "@/web/pages/HomePage";
import DashboardPage from "@/web/pages/DashboardPage";
import GoalsPage from "@/web/pages/GoalsPage";
import ExplorePage from "@/web/pages/ExplorePage";
import UniversityDetailPage from "@/web/pages/UniversityDetailPage";
import ProfilePage from "@/web/pages/ProfilePage";
import BlogPage from "@/web/pages/BlogPage";
import ArticlePage from "@/web/pages/ArticlePage";
import TermsPage from "@/web/pages/TermsPage";
import PrivacyPage from "@/web/pages/PrivacyPage";
import LoginPage from "@/web/pages/auth/login";
import SignUpPage from "@/web/pages/auth/signup";
import ForgotPasswordPage from "@/web/pages/auth/forgot-password";
import ResetPasswordPage from "@/web/pages/auth/reset-password";
import VerifyEmailPage from "@/web/pages/auth/verify-email";
import NotFoundPage from "@/web/pages/NotFoundPage";

const protectedRoute = (element: React.ReactNode) => (
  <RequireAuth>{element}</RequireAuth>
);

export default function App() {
  return (
    <div className="min-h-full flex flex-col">
      <Header />
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

        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </div>
  );
}
