import { sanitizeURL } from "./utils.js";

const DEFAULT_ICON = `
<svg width="32" height="32" viewBox="0 0 24 24" fill="white" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
	<path d="M12 2C6.48 2 2 6.48 2 12C2 17.52 6.48 22 12 22C17.52 22 22 17.52 22 12C22 6.48 17.52 2 12 2ZM12 6C13.66 6 15 7.34 15 9C15 10.66 13.66 12 12 12C10.34 12 9 10.66 9 9C9 7.34 10.34 6 12 6ZM12 19.2C9.5 19.2 7.29 17.92 6 15.98C6.03 13.99 10 12.9 12 12.9C13.99 12.9 17.97 13.99 18 15.98C16.71 17.92 14.5 19.2 12 19.2Z"/>
</svg>`;

export function collectElements(shadowRoot) {
	return {
		widget: shadowRoot.getElementById("textChatWidget"),
		floatingBtn: shadowRoot.getElementById("floating-btn"),
		closeBtn: shadowRoot.getElementById("closeTextChat"),
		messagesContainer: shadowRoot.getElementById("textMessagesArea"),
		input: shadowRoot.getElementById("textMessageInput"),
		inputError: shadowRoot.getElementById("chatInputError"),
		sendBtn: shadowRoot.getElementById("textSendButton"),
		languageSelector: shadowRoot.getElementById("languageSelector"),
		chatIcon: shadowRoot.getElementById("chatIcon"),
		bannerText: shadowRoot.getElementById("bannerText"),
		bannerSubText: shadowRoot.getElementById("bannerSubText"),
		contactFormSlot: shadowRoot.getElementById("contactFormSlot"),
		cfName: shadowRoot.getElementById("cf-name"),
		cfEmail: shadowRoot.getElementById("cf-email"),
		cfMessage: shadowRoot.getElementById("cf-message"),
		cfSubmit: shadowRoot.getElementById("cf-submit"),
		chatInput: shadowRoot.getElementById("chatInput"),
		conversationRatingSlot: shadowRoot.getElementById("conversationRatingSlot"),
	};
}

export function applyTheme(host, config) {
	const primaryColor = config.primaryColor || config.sendColor || "#fc0e3f";
	const backgroundColor = config.backgroundColor || config.bannerColor || "#120b14";
	const textColor = config.textColor || config.bannerTextColor || "#ffffff";
	const subtitleColor = config.bannerTextParagraphColor || config.subtitleColor || "#94a3b8";
	const position = config.position === "bottom-left" ? "bottom-left" : "bottom-right";
	const borderRadius =
		typeof config.borderRadius === "number"
			? `${config.borderRadius}px`
			: String(config.borderRadius || "24px");
	const fontSize =
		typeof config.fontSize === "number" ? `${config.fontSize}px` : String(config.fontSize || "14px");

	const theme = {
		"--kv-banner-color": backgroundColor,
		"--kv-banner-text-color": textColor,
		"--kv-banner-subtext-color": subtitleColor,
		"--kv-user-chat-color": config.userChatColor || "#ffdce4",
		"--kv-send-color": primaryColor,
		"--kv-floating-btn-color": primaryColor,
		"--kv-close-button-color": config.closeButtonColor || "#ffffff",
		"--kv-widget-radius": borderRadius,
		"--kv-widget-font-size": fontSize,
		"--kv-widget-right": position === "bottom-left" ? "auto" : "2em",
		"--kv-widget-left": position === "bottom-left" ? "2em" : "auto",
		"--kv-floating-right": position === "bottom-left" ? "auto" : "45px",
		"--kv-floating-left": position === "bottom-left" ? "45px" : "auto",
	};

	Object.keys(theme).forEach((variable) => {
		host.style.setProperty(variable, theme[variable]);
	});
}

export function applyHeaderContent(elements, config) {
	if (elements.bannerText) {
		elements.bannerText.textContent = config.botName || config.bannerText || "Text Chat";
	}

	if (elements.bannerSubText) {
		const subtitle = (config.welcomeMessage || config.bannerTextParagraph || "").trim();
		elements.bannerSubText.textContent = subtitle;
		elements.bannerSubText.classList.toggle("hidden", !subtitle);
	}
}

export function renderHeaderIcon(container, logoIconURL) {
	if (!container) {
		return;
	}

	const safeLogoURL = sanitizeURL(logoIconURL);
	if (safeLogoURL) {
		container.innerHTML = `<img id="logoIcon" src="${safeLogoURL}" alt="Widget logo" />`;
		return;
	}

	container.innerHTML = DEFAULT_ICON;
}

export function populateLanguageOptions(selectElement, languages, selectedLanguage) {
	if (!selectElement) {
		return;
	}

	selectElement.innerHTML = languages
		.map((language) => {
			const selected = language.code === selectedLanguage ? " selected" : "";
			return `<option value="${language.code}"${selected}>${language.label}</option>`;
		})
		.join("");
}

export function setInputMaxLength(input, maxLength) {
	if (input && Number.isFinite(maxLength) && maxLength > 0) {
		input.maxLength = maxLength;
	}
}
