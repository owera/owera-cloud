/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  // Status page must never depend on API auth or runtime config it cannot
  // see at build time; expose only NEXT_PUBLIC_* envs.
};

export default nextConfig;
