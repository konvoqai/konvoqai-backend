import { KonvoqChatWidget } from "./widget/KonvoqChatWidget.js";

if (!customElements.get("konvoq-chat")) {
	customElements.define("konvoq-chat", KonvoqChatWidget);
}
