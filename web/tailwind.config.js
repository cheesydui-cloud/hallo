/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js}'],
  theme: {
    extend: {
      colors: {
        ink: '#0f1419',
        paper: '#f4efe6',
        card: '#fffdf8',
        accent: '#c45c26',
        pine: '#2f5d50',
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        display: ['"Fraunces"', '"Noto Serif SC"', 'serif'],
      },
    },
  },
  plugins: [],
}
