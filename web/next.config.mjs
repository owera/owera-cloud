/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  // We proxy /api/proxy/* to the upstream Owera API; nothing else is rewritten.
  experimental: {
    // Keep this minimal — App Router defaults are fine for the scaffold.
  },
};

export default nextConfig;
