import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The dev server proxies the versioned API to the Go backend so the frontend
// can use same-origin image URLs (which keeps node-vibrant's canvas reads
// CORS-clean).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_TARGET || "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});