import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react()],
  base: "/app/",
  build: {
    outDir: fileURLToPath(new URL("../internal/web/react-dist", import.meta.url)),
    emptyOutDir: true,
    sourcemap: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/static": "http://127.0.0.1:8080",
      "/products": "http://127.0.0.1:8080",
      "/purchases": "http://127.0.0.1:8080",
      "/sales": "http://127.0.0.1:8080",
      "/shipments": "http://127.0.0.1:8080",
    },
  },
});
