import { defineConfig, loadEnv } from "vite-plus";
import { viteSingleFile } from "vite-plugin-singlefile";

// process.envはconfig評価時点でfront/.envを含まないため、loadEnvで明示的に読む
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, import.meta.dirname, "");
  const apiTarget = env.VITE_API_TARGET;
  return {
    plugins: [viteSingleFile()],
    server: apiTarget
      ? {
          proxy: {
            "/api": {
              target: apiTarget,
              changeOrigin: true,
            },
          },
        }
      : undefined,
  };
});
