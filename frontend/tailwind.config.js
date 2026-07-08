/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        wings: {
          page: "#000000",
          text: "#fbfbfb",
          card: "#1c1c1e",
          surface: "#242427",
          border: "rgba(255, 255, 255, 0.06)",
          divider: "rgba(255, 255, 255, 0.08)",
          muted: "rgba(252, 252, 252, 0.62)",
          mutedStrong: "rgba(252, 252, 252, 0.78)",
          kicker: "rgba(252, 252, 252, 0.55)",
          accent: "#1259d1",
          accentHover: "#1063e6",
          inputLine: "rgba(255, 255, 255, 0.32)",
          secondary: "rgba(255, 255, 255, 0.08)",
          chip: "rgba(255, 255, 255, 0.06)",
          badgeBg: "rgba(140, 168, 255, 0.14)",
          badgeText: "#b7c8ff",
          input: "rgba(0, 0, 0, 0.22)",
          danger: "#ff7a8c",
        },
      },
      fontFamily: {
        samsung: ["SamsungOne", "Segoe UI", "Roboto", "sans-serif"],
        sharp: ["SamsungSharpSans", "SamsungOne", "sans-serif"],
      },
      boxShadow: {
        card: "0 16px 60px rgba(0, 0, 0, 0.34), inset 0 1px 0 rgba(255, 255, 255, 0.04)",
      },
      backdropBlur: {
        18: "18px",
      },
      animation: {
        "oneui-loader": "oneui-loader-spin 0.82s linear infinite",
        "oneui-background": "oneui-background-drift 18s ease-in-out infinite alternate",
      },
    },
  },
  plugins: [],
};
