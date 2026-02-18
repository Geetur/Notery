/** @type {import('next').NextConfig} */
const nextConfig = {
    webpack: (config) => {
        // react-pdf / pdfjs-dist v5 requires canvas and encoding in Node-side contexts.
        // Mark them as false so Next.js doesn't try to bundle/polyfill them.
        config.resolve.alias.canvas = false;
        config.resolve.alias.encoding = false;
        return config;
    },
};

export default nextConfig;
