import { Route, Routes } from "react-router";
import Header from "@/web/components/Header";
import RequireAuth from "@/web/components/RequireAuth";
import HomePage from "@/web/pages/HomePage";
import DashboardPage from "@/web/pages/DashboardPage";
import GoalsPage from "@/web/pages/GoalsPage";
import ExplorePage from "@/web/pages/ExplorePage";
import UniversityDetailPage from "@/web/pages/UniversityDetailPage";
import ProfilePage from "@/web/pages/ProfilePage";
import NotFoundPage from "@/web/pages/NotFoundPage";

export default function App() {
  return (
    <div className="min-h-full flex flex-col">
      <Header />
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route
          path="/dashboard"
          element={
            <RequireAuth>
              <DashboardPage />
            </RequireAuth>
          }
        />
        <Route
          path="/goals"
          element={
            <RequireAuth>
              <GoalsPage />
            </RequireAuth>
          }
        />
        <Route
          path="/explore"
          element={
            <RequireAuth>
              <ExplorePage />
            </RequireAuth>
          }
        />
        <Route
          path="/explore/:universityId"
          element={
            <RequireAuth>
              <UniversityDetailPage />
            </RequireAuth>
          }
        />
        <Route
          path="/profile"
          element={
            <RequireAuth>
              <ProfilePage />
            </RequireAuth>
          }
        />
        {/* Next.js 版と同じく /schedule はダッシュボードのカレンダーへ寄せる */}
        <Route path="/schedule" element={<DashboardPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </div>
  );
}
