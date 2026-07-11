import { lazy, StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { DocsApp } from "./docs/DocsApp";
import "./index.css";

const DesignLabApp = lazy(() => import("./design-lab/DesignLabApp"));
const Root = window.location.pathname.startsWith("/docs")
  ? DocsApp
  : window.location.pathname.startsWith("/designs")
    ? DesignLabApp
    : App;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Suspense fallback={<div className="route-loading">Opening workspace…</div>}>
      <Root />
    </Suspense>
  </StrictMode>,
);
