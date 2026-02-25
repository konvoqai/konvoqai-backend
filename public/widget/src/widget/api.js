export function buildStreamWebhookURL(apiURL) {
	if (!apiURL) {
		return "";
	}
	return apiURL.includes("?") ? `${apiURL}&stream=1` : `${apiURL}?stream=1`;
}

export async function postJSON(url, payload) {
	return fetch(url, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(payload),
	});
}

export async function consumeSSE(response, handlers) {
	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffer = "";

	const processEventLine = (line) => {
		if (!line.startsWith("data: ")) {
			return;
		}
		const jsonPayload = line.slice(6).trim();
		if (!jsonPayload) {
			return;
		}
		try {
			const parsed = JSON.parse(jsonPayload);
			if (handlers && typeof handlers.onEvent === "function") {
				handlers.onEvent(parsed);
			}
		} catch (_error) {
			// Ignore parse errors from malformed stream chunks.
		}
	};

	while (true) {
		const { value, done } = await reader.read();
		if (done) {
			break;
		}

		buffer += decoder.decode(value, { stream: true });
		const events = buffer.split("\n\n");
		buffer = events.pop() || "";

		for (const eventText of events) {
			for (const line of eventText.split("\n")) {
				processEventLine(line);
			}
		}
	}

	if (buffer.trim().startsWith("data:")) {
		const finalLine = buffer.replace(/^data:\s*/, "").trim();
		if (finalLine) {
			try {
				const parsed = JSON.parse(finalLine);
				if (handlers && typeof handlers.onEvent === "function") {
					handlers.onEvent(parsed);
				}
			} catch (_error) {
				// Ignore final line parse issues.
			}
		}
	}
}
