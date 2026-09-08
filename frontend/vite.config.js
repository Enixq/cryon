/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
    plugins: [react()],
    server: {
        host: '0.0.0.0',
        port: 34115,
        strictPort: true,
    },
    // @ts-expect-error Vite's config type does not include Vitest options.
    test: {
        globals: true,
        environment: "jsdom",
        include: ["src/**/*.{test,spec}.{ts,tsx}"],
    },
});
