import { defineConfig } from "vite";
import solid from "vite-plugin-solid";

export default defineConfig({
  plugins: [solid()],
  base: "/",
  server: {
    proxy: {
      "/v1": "http://localhost:8080",
      "/admin/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
      "/files": "http://localhost:8080",
    },
  },
  build: { outDir: "dist/app", emptyOutDir: true },
});
