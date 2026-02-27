import { buildStreamWebhookURL, consumeSSE, postJSON } from "./api.js";
import { CONFIG_ATTRIBUTES, DEFAULT_CONFIG, SUPPORTED_LANGUAGES } from "./constants.js";
import {
	applyHeaderContent,
	applyTheme,
	collectElements,
	populateLanguageOptions,
	renderHeaderIcon,
	setInputMaxLength,
} from "./dom.js";
import {
	appendMessage,
	renderConversationRating,
	showTypingIndicator,
	scrollMessagesToBottom,
	updateTypingToMessage,
} from "./messages.js";
import {
	getRatingShownState,
	getRatingSubmittedState,
	initializeSessionState,
	loadLanguagePreference,
	persistChatCount,
	persistChatDate,
	persistSessionId,
	saveLanguagePreference,
	setRatingShownState,
	setRatingSubmittedState,
} from "./session.js";
import { escapeHtml, normalizeLanguageCode, parseBooleanLike, parseMarkdown, sanitizeHTML, toCamelCaseAttribute } from "./utils.js";
import { mountWidgetTemplate } from "./template.js";

export class KonvoqChatWidget extends HTMLElement {
	constructor() {
		super();
		this.attachShadow({ mode: "open" });

		this.apiUrl = "";
		this.apiBaseUrl = "";
		this.widgetKey = "";
		this.sessionId = "";
		this.isOpen = false;
		this.successfulChatCount = 0;
		this.userMessageCount = 0;
		this.botMessageCount = 0;
		this.pendingEndIntentRating = false;
		this.ratingShown = false;
		this.ratingSubmitted = false;
		this.selectedLanguage = "en";
		this.maxMessageLength = 1000;
		this.date = new Date();
		this._contactEventsBound = false;
		this._initialized = false;
		this._messageHandler = (event) => this.handlePreviewMessage(event);
		this.elements = {};

		this.supportedLanguages = [...SUPPORTED_LANGUAGES];
		this.config = { ...DEFAULT_CONFIG };
	}

	connectedCallback() {
		window.addEventListener("message", this._messageHandler);
		if (this._initialized) {
			return;
		}
		this._initialized = true;
		void this.initialize();
	}

	disconnectedCallback() {
		window.removeEventListener("message", this._messageHandler);
	}

	async initialize() {
		this.initializeSession();
		this.readAttributes();
		this.initializeLanguagePreference();
		await this.loadRemoteConfig();

		await this.render();
		this.bindEvents();

		if (this.config.primaryText) {
			setTimeout(() => this.displayDefaultMessage(), 500);
		}

		if (this.config.autoOpen) {
			setTimeout(() => {
				if (!this.isOpen) {
					this.toggleChat();
				}
			}, 5000);
		}

		setTimeout(() => {
			if (this.elements.floatingBtn) {
				this.elements.floatingBtn.classList.remove("hidden");
			}
		}, 2000);
	}

	initializeSession() {
		const sessionState = initializeSessionState();
		this.date = sessionState.date;
		this.successfulChatCount = sessionState.successfulChatCount;
		this.sessionId = sessionState.sessionId;
		this.ratingShown = getRatingShownState(this.sessionId);
		this.ratingSubmitted = getRatingSubmittedState(this.sessionId);
	}

	readAttributes() {
		this.apiUrl = this.getAttribute("api-url") || "";
		this.apiBaseUrl = this.getAttribute("api-base-url") || this.apiUrl.replace("/api/v1/webhook", "");
		this.widgetKey = this.getAttribute("widget-key") || "";

		for (const attribute of CONFIG_ATTRIBUTES) {
			const rawValue = this.getAttribute(attribute);
			if (rawValue === null) {
				continue;
			}
			const key = toCamelCaseAttribute(attribute);
			this.config[key] = parseBooleanLike(rawValue);
		}

		const maxLengthAttr = Number.parseInt(this.getAttribute("max-message-length") || "", 10);
		if (Number.isFinite(maxLengthAttr) && maxLengthAttr > 0) {
			this.maxMessageLength = maxLengthAttr;
		}
	}

	initializeLanguagePreference() {
		const configuredLanguage = normalizeLanguageCode(this.config.defaultLanguage, this.supportedLanguages) || "en";
		const hasConfiguredAttribute = this.getAttribute("default-language") !== null;
		this.selectedLanguage = loadLanguagePreference(this.widgetKey, configuredLanguage, hasConfiguredAttribute);
		this.config.defaultLanguage = this.selectedLanguage;
	}

	async loadRemoteConfig() {
		if (!this.widgetKey || !this.apiBaseUrl) {
			return;
		}
		try {
			const response = await fetch(`${this.apiBaseUrl}/api/v1/widget/config/${encodeURIComponent(this.widgetKey)}`);
			if (!response.ok) {
				return;
			}
			const payload = await response.json();
			const remoteSettings = payload?.widget?.settings || {};
			if (!remoteSettings || typeof remoteSettings !== "object") {
				return;
			}
			const mapped = this.normalizePreviewConfig(remoteSettings);
			this.config = { ...this.config, ...remoteSettings, ...mapped };
		} catch (_error) {
			// Keep local/default config when remote config is unavailable.
		}
	}

	async render() {
		await mountWidgetTemplate(this.shadowRoot);
		this.elements = collectElements(this.shadowRoot);

		applyTheme(this, this.config);
		applyHeaderContent(this.elements, this.config);
		renderHeaderIcon(this.elements.chatIcon, this.config.logoIcon);
		populateLanguageOptions(this.elements.languageSelector, this.supportedLanguages, this.selectedLanguage);
		setInputMaxLength(this.elements.input, this.maxMessageLength);
	}

	bindEvents() {
		if (this.elements.floatingBtn) {
			this.elements.floatingBtn.addEventListener("click", () => this.toggleChat());
		}
		if (this.elements.closeBtn) {
			this.elements.closeBtn.addEventListener("click", () => this.toggleChat());
		}
		if (this.elements.sendBtn) {
			this.elements.sendBtn.addEventListener("click", () => this.handleSend());
		}
		if (this.elements.input) {
			this.elements.input.addEventListener("input", () => this.clearInputError());
			this.elements.input.addEventListener("keydown", (event) => {
				if (event.key === "Enter" && !event.shiftKey) {
					event.preventDefault();
					this.handleSend();
				}
			});
		}

		if (this.elements.languageSelector) {
			this.elements.languageSelector.addEventListener("change", (event) => {
				const selected = normalizeLanguageCode(event.target.value, this.supportedLanguages) || "en";
				this.selectedLanguage = selected;
				this.config.defaultLanguage = selected;
				saveLanguagePreference(this.widgetKey, selected);
			});
		}
	}

	toggleChat() {
		if (!this.elements.widget || !this.elements.floatingBtn) {
			return;
		}

		if (!this.isOpen) {
			this.isOpen = true;
			this.elements.widget.classList.remove("hidden", "minimizing");
			this.elements.floatingBtn.classList.add("hidden");
			setTimeout(() => {
				if (this.elements.input) {
					this.elements.input.focus();
				}
			}, 100);
			return;
		}

		this.isOpen = false;
		this.elements.widget.classList.add("minimizing");
		setTimeout(() => {
			this.elements.widget.classList.add("hidden");
			this.elements.floatingBtn.classList.remove("hidden");
		}, 300);
	}

	showInputError(message) {
		if (!this.elements.inputError) {
			return;
		}
		this.elements.inputError.textContent = message || "";
		this.elements.inputError.classList.remove("hidden");
	}

	clearInputError() {
		if (!this.elements.inputError) {
			return;
		}
		this.elements.inputError.textContent = "";
		this.elements.inputError.classList.add("hidden");
	}

	resetConversationRatingState() {
		this.pendingEndIntentRating = false;
		this.ratingShown = false;
		this.ratingSubmitted = false;
		setRatingShownState(this.sessionId, false);
		setRatingSubmittedState(this.sessionId, false);
		if (this.elements.conversationRatingSlot) {
			this.elements.conversationRatingSlot.innerHTML = "";
			this.elements.conversationRatingSlot.classList.add("hidden");
		}
	}

	isConversationEndMessage(text) {
		if (!text) {
			return false;
		}

		const normalized = String(text).toLowerCase().trim();
		if (!normalized) {
			return false;
		}

		const endPatterns = [
			/\b(thanks|thank you|thankyou|thx)\b/,
			/\b(bye|goodbye|see you|see ya|take care)\b/,
			/\b(that'?s all|thats all|done|resolved|got it)\b/,
			/\b(no thanks|no thank you|i'?m good|im good)\b/,
		];

		return endPatterns.some((pattern) => pattern.test(normalized));
	}

	renderMessageContent(text, messageType) {
		const raw = String(text || "");
		const safeRaw = escapeHtml(raw);
		if (messageType === "user") {
			return parseMarkdown(safeRaw);
		}
		return parseMarkdown(safeRaw);
	}

	async handleSend() {
		if (!this.elements.input || !this.apiUrl) {
			this.showInputError("Widget is not configured with an API URL.");
			return;
		}

		const rawText = this.elements.input.value || "";
		if (rawText.length > this.maxMessageLength) {
			this.showInputError(`Message is too long. Maximum ${this.maxMessageLength} characters.`);
			return;
		}

		const text = rawText.trim();
		if (!text) {
			this.clearInputError();
			return;
		}
		this.clearInputError();

		const inactivityMs = new Date().getTime() - this.date.getTime();
		if (inactivityMs > 2 * 60 * 1000) {
			this.successfulChatCount = 0;
			this.userMessageCount = 0;
			this.botMessageCount = 0;
			this.date = new Date();
			persistChatDate(this.date);
			this.resetConversationRatingState();
		}

		appendMessage(this.elements.messagesContainer, text, "user", (value) => this.renderMessageContent(value, "user"));
		this.userMessageCount += 1;
		this.pendingEndIntentRating =
			this.config.planType === "basic" && this.isConversationEndMessage(text) && !this.ratingShown && !this.ratingSubmitted;
		this.elements.input.value = "";

		const typingWrapper = showTypingIndicator(this.elements.messagesContainer);

		try {
			const webhookURL = buildStreamWebhookURL(this.apiUrl);
			const response = await postJSON(webhookURL, {
				widgetKey: this.widgetKey,
				message: text,
				sessionId: this.sessionId,
				language: this.selectedLanguage,
			});

			const contentType = (response.headers.get("content-type") || "").toLowerCase();
			if (response.ok && response.body && contentType.includes("text/event-stream")) {
				const streamed = await this.consumeStreamedResponse(response, typingWrapper);
				if (streamed.completed) {
					this.successfulChatCount += 1;
					persistChatCount(this.successfulChatCount);
				}
				return;
			}

			const responseText = await response.text();
			if (!response.ok) {
				let fallbackMessage = "Sorry, did not get that.";
				try {
					const parsedError = JSON.parse(responseText);
					if (parsedError.limitReached && parsedError.data && parsedError.data.planType === "basic") {
						updateTypingToMessage(this.elements.messagesContainer, typingWrapper, "You've reached the conversation limit. Please use the form below to get in touch.", (value) => this.renderMessageContent(value, "bot"));
						this.pendingEndIntentRating = false;
						this.showContactForm();
						return;
					}
					fallbackMessage = parsedError.message || fallbackMessage;
				} catch (_error) {
					// Ignore JSON parse failures.
				}

				updateTypingToMessage(this.elements.messagesContainer, typingWrapper, fallbackMessage, (value) => this.renderMessageContent(value, "bot"));
				this.pendingEndIntentRating = false;
				return;
			}

			let finalMessage = "Sorry, did not get that.";
			try {
				const parsed = JSON.parse(responseText);
				finalMessage = parsed.response || parsed.output || parsed.message || finalMessage;
				if (parsed.sessionId) {
					this.sessionId = parsed.sessionId;
					persistSessionId(parsed.sessionId);
					this.ratingShown = getRatingShownState(this.sessionId);
					this.ratingSubmitted = getRatingSubmittedState(this.sessionId);
				}
			} catch (_error) {
				// Keep fallback message.
			}

			this.successfulChatCount += 1;
			persistChatCount(this.successfulChatCount);
			this.appendBotReply(typingWrapper, finalMessage);
		} catch (_error) {
			updateTypingToMessage(this.elements.messagesContainer, typingWrapper, "Sorry, network error occurred.", (value) => this.renderMessageContent(value, "bot"));
			this.pendingEndIntentRating = false;
		}
	}

	async consumeStreamedResponse(response, typingWrapper) {
		let assembled = "";
		let donePayload = null;
		let streamHadError = false;

		await consumeSSE(response, {
			onEvent: (payload) => {
				if (!payload || !payload.type) {
					return;
				}
				if (payload.type === "token" && typeof payload.token === "string") {
					assembled += payload.token;
					updateTypingToMessage(this.elements.messagesContainer, typingWrapper, assembled, (value) => this.renderMessageContent(value, "bot"));
					return;
				}
				if (payload.type === "done") {
					donePayload = payload;
					return;
				}
				if (payload.type === "error") {
					streamHadError = true;
				}
			},
		});

		if (streamHadError) {
			updateTypingToMessage(this.elements.messagesContainer, typingWrapper, "Sorry, network error occurred.", (value) => this.renderMessageContent(value, "bot"));
			this.pendingEndIntentRating = false;
			return { completed: false };
		}

		if (donePayload && donePayload.sessionId) {
			this.sessionId = donePayload.sessionId;
			persistSessionId(donePayload.sessionId);
			this.ratingShown = getRatingShownState(this.sessionId);
			this.ratingSubmitted = getRatingSubmittedState(this.sessionId);
		}

		if (!assembled.trim()) {
			updateTypingToMessage(this.elements.messagesContainer, typingWrapper, "Sorry, did not get that.", (value) => this.renderMessageContent(value, "bot"));
			this.pendingEndIntentRating = false;
			return { completed: false };
		}

		this.appendBotReply(typingWrapper, assembled);
		return { completed: true };
	}

	appendBotReply(typingWrapper, text) {
		updateTypingToMessage(this.elements.messagesContainer, typingWrapper, text, (value) => this.renderMessageContent(value, "bot"));
		this.botMessageCount += 1;

		const shouldShowConversationRating =
			this.config.planType === "basic" &&
			this.pendingEndIntentRating &&
			!this.ratingShown &&
			!this.ratingSubmitted &&
			this.userMessageCount > 0 &&
			this.botMessageCount > 0;

		if (shouldShowConversationRating && this.elements.conversationRatingSlot) {
			renderConversationRating(this.elements.conversationRatingSlot, (rating) => {
				this.elements.conversationRatingSlot.innerHTML = "";
				this.elements.conversationRatingSlot.classList.add("hidden");
				this.submitRating(rating);
			});
			scrollMessagesToBottom(this.elements.messagesContainer);
			this.ratingShown = true;
			setRatingShownState(this.sessionId, true);
		}

		this.pendingEndIntentRating = false;
	}

	async submitRating(rating) {
		if (!this.apiBaseUrl) {
			return;
		}
		try {
			await postJSON(`${this.apiBaseUrl}/api/v1/widget/rating`, {
				widgetKey: this.widgetKey,
				sessionId: this.sessionId,
				rating,
			});
		} catch (_error) {
			// Non-fatal if analytics endpoint fails.
		}

		this.ratingSubmitted = true;
		setRatingSubmittedState(this.sessionId, true);
		if (this.elements.conversationRatingSlot) {
			this.elements.conversationRatingSlot.innerHTML = "";
			this.elements.conversationRatingSlot.classList.add("hidden");
		}
	}

	showContactForm() {
		if (!this.elements.contactFormSlot) {
			return;
		}

		if (this.elements.messagesContainer) {
			this.elements.messagesContainer.classList.add("hidden");
		}
		if (this.elements.chatInput) {
			this.elements.chatInput.classList.add("hidden");
		}
		this.elements.contactFormSlot.classList.remove("hidden");

		if (!this._contactEventsBound && this.elements.cfSubmit) {
			this._contactEventsBound = true;
			this.elements.cfSubmit.addEventListener("click", () => this.submitContactForm());
		}
	}

	async submitContactForm() {
		const email = this.elements.cfEmail ? this.elements.cfEmail.value.trim() : "";
		if (!email) {
			if (this.elements.cfEmail) {
				this.elements.cfEmail.style.borderColor = "#ef4444";
			}
			return;
		}

		if (this.elements.cfSubmit) {
			this.elements.cfSubmit.disabled = true;
			this.elements.cfSubmit.textContent = "Sending...";
		}

		try {
			const response = await postJSON(`${this.apiBaseUrl}/api/v1/widget/contact`, {
				widgetKey: this.widgetKey,
				sessionId: this.sessionId,
				name: this.elements.cfName ? this.elements.cfName.value.trim() || null : null,
				email,
				message: this.elements.cfMessage ? this.elements.cfMessage.value.trim() || null : null,
			});

			if (response.ok) {
				this.elements.contactFormSlot.innerHTML = '<div class="contact-form-success">Message sent. We will be in touch soon.</div>';
				return;
			}
		} catch (_error) {
			// Fall through to reset submit state.
		}

		if (this.elements.cfSubmit) {
			this.elements.cfSubmit.disabled = false;
			this.elements.cfSubmit.textContent = "Send Message";
		}
	}

	displayDefaultMessage() {
		if (!this.config.primaryText || !this.elements.messagesContainer) {
			return;
		}
		appendMessage(this.elements.messagesContainer, sanitizeHTML(this.config.primaryText), "bot", (value) =>
			this.renderMessageContent(value, "bot"),
		);
	}

	handlePreviewMessage(event) {
		const payload = event?.data;
		if (!payload || payload.type !== "konvoq:widget-config" || typeof payload.config !== "object") {
			return;
		}
		const mapped = this.normalizePreviewConfig(payload.config);
		this.applyRuntimeConfig(mapped);
	}

	normalizePreviewConfig(config) {
		return {
			primaryColor: config.primaryColor,
			backgroundColor: config.backgroundColor,
			textColor: config.textColor,
			botName: config.botName,
			welcomeMessage: config.welcomeMessage,
			logoIcon: config.logoUrl || config.logoIcon,
			position: config.position,
			borderRadius: config.borderRadius,
			fontSize: config.fontSize,
			primaryText: config.welcomeMessage,
		};
	}

	applyRuntimeConfig(nextConfig) {
		this.config = { ...this.config, ...nextConfig };
		if (!this.elements || !this.elements.widget) {
			return;
		}
		applyTheme(this, this.config);
		applyHeaderContent(this.elements, this.config);
		renderHeaderIcon(this.elements.chatIcon, this.config.logoIcon);
	}
}
