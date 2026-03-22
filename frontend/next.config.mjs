/** @type {import('next').NextConfig} */
const nextConfig = {
    output: "standalone",
    webpack: (config) => {
        // react-pdf / pdfjs-dist requires canvas and encoding in Node-side
        // contexts. Mark them as false so Next.js doesn't try to bundle them.
        config.resolve.alias.canvas = false;
        config.resolve.alias.encoding = false;
        return config;
    },
    async headers() {
        return [
            {
                source: "/(.*)",
                headers: [
                    {
                        key: "X-Content-Type-Options",
                        value: "nosniff",
                    },
                    {
                        key: "X-Frame-Options",
                        value: "DENY",
                    },
                    {
                        key: "Referrer-Policy",
                        value: "strict-origin-when-cross-origin",
                    },
                    {
                        key: "Permissions-Policy",
                        value: "camera=(), microphone=(), geolocation=()",
                    },
                    {
                        key: "Content-Security-Policy",
                        value: [
                            "default-src 'self'",
                            "script-src 'self' 'unsafe-eval' 'unsafe-inline'",
                            "style-src 'self' 'unsafe-inline'",
                            "img-src 'self' data: blob: " + (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"),
                            "font-src 'self'",
                            "connect-src 'self' " + (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"),
                            "worker-src 'self' blob:",
                            "frame-src 'none'",
                        ].join("; "),
                    },
                ],
            },
        ];
    },
};

export default nextConfig;
