import { Routes, Route } from "react-router-dom";
import ViewerPage from "./pages/ViewerPage";
import AdminPage from "./pages/AdminPage";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<ViewerPage />} />
      <Route path="/admin" element={<AdminPage />} />
    </Routes>
  );
}
