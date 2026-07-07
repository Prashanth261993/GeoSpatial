import { createRoot } from "react-dom/client";
import { HashRouter, Routes, Route } from "react-router-dom";
import { App } from "./App";
import { Cinematic } from "./Cinematic";

createRoot(document.getElementById("root")!).render(
  <HashRouter>
    <Routes>
      <Route path="/" element={<App />} />
      <Route path="/cinematic" element={<Cinematic />} />
    </Routes>
  </HashRouter>
);
