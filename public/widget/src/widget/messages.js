export function scrollMessagesToBottom(container) {
	if (container) {
		container.scrollTop = container.scrollHeight;
	}
}

export function appendMessage(container, text, type, renderContent) {
	const wrapper = document.createElement("div");
	wrapper.className = `chat-message ${type === "user" ? "user" : ""}`;

	const bubble = document.createElement("div");
	bubble.className = type === "user" ? "chat-bubble-user" : "chat-bubble-ai";
	bubble.innerHTML = `<div class="md-content">${renderContent(text, type)}</div>`;

	wrapper.appendChild(bubble);
	container.appendChild(wrapper);
	scrollMessagesToBottom(container);
	return wrapper;
}

export function showTypingIndicator(container) {
	const wrapper = document.createElement("div");
	wrapper.className = "chat-message";

	const bubble = document.createElement("div");
	bubble.className = "typing-indicator chat-bubble-ai";
	bubble.innerHTML = `
		<div class="typing-container">
			<span class="typing-dots-text">.</span>
			<span class="typing-dots-text">.</span>
			<span class="typing-dots-text">.</span>
		</div>
	`;

	wrapper.appendChild(bubble);
	container.appendChild(wrapper);
	scrollMessagesToBottom(container);
	return wrapper;
}

export function updateTypingToMessage(container, wrapper, text, renderContent) {
	const typingBubble = wrapper.querySelector(".typing-indicator");
	if (typingBubble) {
		typingBubble.classList.remove("typing-indicator");
		typingBubble.innerHTML = `<div class="md-content">${renderContent(text, "bot")}</div>`;
	} else {
		const messageBubble = wrapper.querySelector(".chat-bubble-ai");
		if (messageBubble) {
			messageBubble.innerHTML = `<div class="md-content">${renderContent(text, "bot")}</div>`;
		}
	}

	scrollMessagesToBottom(container);
}

export function renderConversationRating(slot, onSelected) {
	if (!slot) {
		return;
	}

	const ratingRow = document.createElement("div");
	ratingRow.className = "rating-row";
	ratingRow.innerHTML = `
		<span class="rating-label">Rate this conversation</span>
		<button class="rating-btn" data-rating="up" title="Thumbs up">&#128077;</button>
		<button class="rating-btn" data-rating="down" title="Thumbs down">&#128078;</button>
	`;

	ratingRow.querySelectorAll(".rating-btn").forEach((button) => {
		button.addEventListener("click", (event) => {
			const clicked = event.currentTarget;
			const rating = clicked.dataset.rating;
			ratingRow.querySelectorAll(".rating-btn").forEach((item) => item.classList.remove("active"));
			clicked.classList.add("active");
			if (typeof onSelected === "function") {
				onSelected(rating);
			}
		});
	});

	slot.innerHTML = "";
	slot.appendChild(ratingRow);
	slot.classList.remove("hidden");
}
