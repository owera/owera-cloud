import type { Config } from "tailwindcss";

// Tailwind v4 mostly reads design tokens from CSS @theme blocks in globals.css.
// This file stays minimal — content paths + a small `darkMode` knob.
const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  darkMode: "class",
  theme: {
    extend: {},
  },
  plugins: [],
};

export default config;
