// Tailwind CSS build config for the server-rendered UI.
// Build: npx --yes tailwindcss@3 -c tailwind.config.js -i internal/web/static/input.css -o internal/web/static/app.css --minify
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/web/**/*.templ",
    "./internal/web/**/*.go",
  ],
  theme: { extend: {} },
  plugins: [],
};
