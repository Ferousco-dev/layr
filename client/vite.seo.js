// A small Vite plugin: fills the site URL into index.html and writes sitemap.xml and robots.txt at build time.
export default function seo(siteUrl) {
  const site = siteUrl.replace(/\/+$/, '')
  // Browsers cache favicons per host for a long time, so the icon URLs change with every build or dev start.
  const version = Date.now().toString(36)
  const pages = [
    { path: '/', freq: 'monthly', priority: '1.0' },
    { path: '/terms', freq: 'yearly', priority: '0.3' },
    { path: '/privacy', freq: 'yearly', priority: '0.3' },
    { path: '/contact', freq: 'yearly', priority: '0.4' },
  ]
  return {
    name: 'layr-seo',
    transformIndexHtml: (html) => html.replaceAll('__SITE_URL__', site).replaceAll('__ICON_VERSION__', version),
    generateBundle() {
      const day = new Date().toISOString().slice(0, 10)
      this.emitFile({
        type: 'asset',
        fileName: 'sitemap.xml',
        source: `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${pages.map((p) => `  <url>\n    <loc>${site}${p.path}</loc>\n    <lastmod>${day}</lastmod>\n    <changefreq>${p.freq}</changefreq>\n    <priority>${p.priority}</priority>\n  </url>\n`).join('')}</urlset>\n`,
      })
      this.emitFile({
        type: 'asset',
        fileName: 'robots.txt',
        source: `User-agent: *\nAllow: /\n\nSitemap: ${site}/sitemap.xml\n`,
      })
    },
  }
}
