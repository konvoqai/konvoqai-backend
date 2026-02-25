import { SUPPORTED_LANGUAGES } from "./constants.js";

export function sanitizeURL(url) {
	if (!url) {
		return "";
	}
	return /^(https?|mailto|tel):/i.test(url) ? String(url) : "";
}

export function sanitizeHTML(value) {
	const div = document.createElement("div");
	div.textContent = String(value || "");
	return div.innerHTML;
}

export function escapeHtml(value) {
	return String(value || "").replace(/[&<>"']/g, (char) => {
		return {
			"&": "&amp;",
			"<": "&lt;",
			">": "&gt;",
			'"': "&quot;",
			"'": "&#039;",
		}[char];
	});
}

export function parseMarkdown(text) {
	if (!text) {
		return "";
	}

	let html = String(text);

	html = html.replace(/\*\*(.*?)\*\*/g, "<strong>$1</strong>");

	html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, label, url) => {
		return `<a href="${sanitizeURL(url)}" target="_blank" rel="noopener noreferrer">${label}</a>`;
	});

	html = html.replace(/\n/g, "<br>");

	return html;
}

export function normalizeLanguageCode(value, supportedLanguages = SUPPORTED_LANGUAGES) {
	if (typeof value !== "string") {
		return null;
	}
	const normalized = value.trim().toLowerCase();
	if (!normalized) {
		return null;
	}
	return supportedLanguages.some((entry) => entry.code === normalized) ? normalized : null;
}

export function toCamelCaseAttribute(attributeName) {
	return attributeName.replace(/-([a-z])/g, (_match, char) => char.toUpperCase());
}

export function parseBooleanLike(value) {
	if (value === "true") {
		return true;
	}
	if (value === "false") {
		return false;
	}
	return value;
}
