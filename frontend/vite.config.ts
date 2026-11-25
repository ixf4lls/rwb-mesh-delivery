import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
    const { MESH_API_PROXY_TARGET } = loadEnv(mode, '.', 'MESH_API_PROXY_TARGET');
    const target = MESH_API_PROXY_TARGET && new URL(MESH_API_PROXY_TARGET).origin;

    return {
        plugins: [react()],
        build: { assetsInlineLimit: 0 },
        server: {
            proxy: target ? {
                '^/api/v1(?:/|$)': {
                    target,
                    changeOrigin: true,
                    configure: (proxy) => {
                        proxy.on('proxyRes', (response) => {
                            const location = response.headers.location;
                            if (response.statusCode !== 202 || !location || !URL.canParse(location)) return;

                            const operation = new URL(location);
                            if (operation.origin === target && operation.pathname.startsWith('/api/v1/operations/')) {
                                response.headers.location = `${operation.pathname}${operation.search}${operation.hash}`;
                            }
                        });
                    },
                },
            } : undefined,
        },
    };
});
