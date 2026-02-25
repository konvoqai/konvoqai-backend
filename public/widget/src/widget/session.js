import { STORAGE_KEYS } from "./constants.js";
import { normalizeLanguageCode } from "./utils.js";

function getStorage() {
	try {
		return window.sessionStorage;
	} catch (_error) {
		return null;
	}
}

function createSessionId() {
	return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (char) => {
		const random = (Math.random() * 16) | 0;
		const value = char === "x" ? random : (random & 0x3) | 0x8;
		return value.toString(16);
	});
}

export function initializeSessionState() {
	const storage = getStorage();
	const nowIso = new Date().toISOString();
	let date = new Date();
	let successfulChatCount = 0;
	let sessionId = createSessionId();

	if (!storage) {
		return { date, successfulChatCount, sessionId };
	}

	const storedDate = storage.getItem(STORAGE_KEYS.chatDate);
	if (storedDate) {
		date = new Date(storedDate);
	} else {
		storage.setItem(STORAGE_KEYS.chatDate, nowIso);
	}

	const storedCount = storage.getItem(STORAGE_KEYS.chatCount);
	if (storedCount) {
		successfulChatCount = Number(storedCount) || 0;
	} else {
		storage.setItem(STORAGE_KEYS.chatCount, "0");
	}

	const storedSessionId = storage.getItem(STORAGE_KEYS.sessionToken);
	if (storedSessionId) {
		sessionId = storedSessionId;
	} else {
		storage.setItem(STORAGE_KEYS.sessionToken, sessionId);
	}

	return { date, successfulChatCount, sessionId };
}

export function persistChatCount(count) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(STORAGE_KEYS.chatCount, `${count}`);
	}
}

export function persistChatDate(date) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(STORAGE_KEYS.chatDate, date.toISOString());
	}
}

export function persistSessionId(sessionId) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(STORAGE_KEYS.sessionToken, sessionId);
	}
}

export function getRatingShownKey(sessionId) {
	return `konvoq_chat_rating_shown_${sessionId}`;
}

export function getRatingSubmittedKey(sessionId) {
	return `konvoq_chat_rating_submitted_${sessionId}`;
}

export function getRatingShownState(sessionId) {
	const storage = getStorage();
	if (!storage) {
		return false;
	}
	return storage.getItem(getRatingShownKey(sessionId)) === "1";
}

export function getRatingSubmittedState(sessionId) {
	const storage = getStorage();
	if (!storage) {
		return false;
	}
	return storage.getItem(getRatingSubmittedKey(sessionId)) === "1";
}

export function setRatingShownState(sessionId, value) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(getRatingShownKey(sessionId), value ? "1" : "0");
	}
}

export function setRatingSubmittedState(sessionId, value) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(getRatingSubmittedKey(sessionId), value ? "1" : "0");
	}
}

export function getLanguageStorageKey(widgetKey) {
	return `konvoq_chat_language_${widgetKey || "default"}`;
}

export function loadLanguagePreference(widgetKey, configuredLanguage, hasConfiguredAttribute) {
	const storage = getStorage();
	if (!storage) {
		return configuredLanguage || "en";
	}

	const storageKey = getLanguageStorageKey(widgetKey);
	const storedLanguage = normalizeLanguageCode(storage.getItem(storageKey));
	const selectedLanguage = hasConfiguredAttribute
		? configuredLanguage || "en"
		: storedLanguage || configuredLanguage || "en";

	storage.setItem(storageKey, selectedLanguage);
	return selectedLanguage;
}

export function saveLanguagePreference(widgetKey, languageCode) {
	const storage = getStorage();
	if (storage) {
		storage.setItem(getLanguageStorageKey(widgetKey), languageCode);
	}
}
