import path from "path"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
  },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return;
          if (
            id.includes('/recharts') ||
            id.includes('/d3-') ||
            id.includes('/recharts-')
          ) {
            return 'recharts';
          }
          if (
            id.includes('/framer-motion') ||
            id.includes('/gsap') ||
            id.includes('/@gsap/')
          ) {
            return 'motion';
          }
        },
      },
    },
  },
});
