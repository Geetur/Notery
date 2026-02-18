/** @type {import('next').NextConfig} */
const nextConfig = {
    webpack: (config) => {
        // react-pdf / pdfjs-dist requires canvas in some Node-side contexts.
        // Mark it as external so Next.js doesn't try to bundle it.
        config.resolve.alias.canvas = false;
        return config;
    },
};

export default nextConfig;
