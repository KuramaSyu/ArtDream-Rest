import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { AuthProvider } from "./auth/AuthContext";
import { FrontendConfigProvider } from "./config/FrontendConfig";
import { FeedbackProvider } from "./components/Feedback";
import { ThemeHost } from "./theme/ThemeHost";
import "./styles/global.css";

const container = document.getElementById("root");
if (!container) throw new Error("Root element missing");

createRoot(container).render(
  <StrictMode>
    <FeedbackProvider>
      <AuthProvider>
        <FrontendConfigProvider>
          <ThemeHost>
            <BrowserRouter>
              <App />
            </BrowserRouter>
          </ThemeHost>
        </FrontendConfigProvider>
      </AuthProvider>
    </FeedbackProvider>
  </StrictMode>,
);