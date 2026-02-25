(function () {
	"use strict";

	if (window.__KONVOQ_WIDGET_BOOTSTRAPPED__) {
		return;
	}
	window.__KONVOQ_WIDGET_BOOTSTRAPPED__ = true;

	const currentScript = document.currentScript;
	let baseURL = "";

	if (currentScript && currentScript.src) {
		baseURL = new URL(".", currentScript.src).toString();
	} else {
		const scripts = document.getElementsByTagName("script");
		const fallbackScript = scripts[scripts.length - 1];
		if (fallbackScript && fallbackScript.src) {
			baseURL = new URL(".", fallbackScript.src).toString();
		} else {
			baseURL = new URL("./", window.location.href).toString();
		}
	}

	window.__KONVOQ_WIDGET_BASE_URL__ = baseURL;

	const moduleScript = document.createElement("script");
	moduleScript.type = "module";
	moduleScript.async = true;
	moduleScript.src = new URL("./src/index.js", baseURL).toString();
	document.head.appendChild(moduleScript);
})();
