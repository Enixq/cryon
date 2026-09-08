/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'SF Pro Display', 'Google Sans', 'ui-sans-serif', 'system-ui', 'sans-serif'],
      },
      // Токены CRYON DESIGN SYSTEM. Имена подобраны так, чтобы НЕ переопределять
      // дефолты Tailwind (например, rounded-lg): 16px = rounded-2xl уже есть.
      // Значения — ссылки на CSS-переменные из styles.css (единый источник).
      colors: {
        surface: {
          0: 'var(--bg-0)',
          1: 'var(--bg-1)',
          2: 'var(--bg-2)',
        },
        neon: {
          DEFAULT: 'var(--accent)',
          soft: 'var(--accent-soft)',
        },
      },
      borderRadius: {
        card: '16px',
        cryon: '20px',
        pill: '28px',
      },
      boxShadow: {
        glow: '0 0 45px rgba(124, 80, 255, 0.22)',
      },
    },
  },
  plugins: [],
};
