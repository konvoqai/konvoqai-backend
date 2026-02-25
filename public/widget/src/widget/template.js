const HTML_TEMPLATE_URL = new URL("../../templates/widget.html", import.meta.url).toString();
const CSS_TEMPLATE_URL = new URL("../../templates/widget.css", import.meta.url).toString();

let templatePromise = null;

async function fetchText(url) {
	const response = await fetch(url);
	if (!response.ok) {
		throw new Error(`Failed to fetch template asset: ${url}`);
	}
	return response.text();
}

function getFallbackShell() {
	return {
		html: '<div class="widget-fallback">Failed to load widget template assets.</div>',
		css: ".widget-fallback{padding:12px;font-family:sans-serif;font-size:12px;color:#991b1b;background:#fee2e2;border:1px solid #fecaca;border-radius:8px;}",
	};
}

async function loadTemplateAssets() {
	if (!templatePromise) {
		templatePromise = Promise.all([fetchText(HTML_TEMPLATE_URL), fetchText(CSS_TEMPLATE_URL)])
			.then(([html, css]) => ({ html, css }))
			.catch((_error) => getFallbackShell());
	}
	return templatePromise;
}

export async function mountWidgetTemplate(shadowRoot) {
	const shell = await loadTemplateAssets();
	shadowRoot.innerHTML = `<style>${shell.css}</style>${shell.html}`;
}
