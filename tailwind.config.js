/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './internal/**/*.go',
    './web/static/js/*.js',
  ],
  theme: { extend: {} },
  plugins: [],
}
