import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ToastProvider } from "./components/Toast.js";
import { PlayerDragProvider } from "./PlayerDragContext.js";
import { App } from "./App.js";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ToastProvider>
      <PlayerDragProvider>
        <App />
      </PlayerDragProvider>
    </ToastProvider>
  </StrictMode>
);
