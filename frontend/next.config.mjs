/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,

  // The API base is read at build time on the server and inlined into the
  // client bundle, so a deployment can point the same image at any backend.
  env: {
    NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1",
    NEXT_PUBLIC_WS_URL: process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8080/api/v1/ws",
  },

  images: {
    // Uploads are served from S3/MinIO, which is a different origin in every
    // environment, so the host is configured rather than hard-coded.
    remotePatterns: [
      { protocol: "http", hostname: "127.0.0.1" },
      { protocol: "http", hostname: "localhost" },
      { protocol: "https", hostname: "**.loadify.uz" },
    ],
  },

  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        ],
      },
    ];
  },
};

export default nextConfig;
