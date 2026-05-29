/**
 * OpenAI-compatible streaming and non-streaming chat response handler.
 */
(function () {
  "use strict";

  function extractDelta(obj) {
    if (!obj || !obj.choices || !obj.choices.length) return "";
    var c0 = obj.choices[0];
    if (c0.delta && c0.delta.content != null) return String(c0.delta.content);
    if (c0.message && c0.message.content != null) return String(c0.message.content);
    return "";
  }

  function extractMessageContent(obj) {
    if (!obj || !obj.choices || !obj.choices.length) return "";
    var c0 = obj.choices[0];
    if (c0.message && c0.message.content != null) return String(c0.message.content);
    return "";
  }

  function parseErrorBody(obj) {
    if (!obj) return "Request failed";
    if (obj.error) {
      if (typeof obj.error === "string") return obj.error;
      if (obj.error.message) return String(obj.error.message);
    }
    return "Request failed";
  }

  function consumeStream(res, onDelta) {
    var reader = res.body && res.body.getReader ? res.body.getReader() : null;
    if (!reader) return Promise.reject(new Error("Streaming not supported"));

    var decoder = new TextDecoder();
    var buffer = "";

    function pump() {
      return reader.read().then(function (chunk) {
        if (chunk.done) return;
        buffer += decoder.decode(chunk.value, { stream: true });
        var parts = buffer.split("\n");
        buffer = parts.pop() || "";
        for (var i = 0; i < parts.length; i++) {
          var line = parts[i].trim();
          if (!line || line.indexOf("data:") !== 0) continue;
          var payload = line.slice(5).trim();
          if (payload === "[DONE]") continue;
          try {
            var obj = JSON.parse(payload);
            var delta = extractDelta(obj);
            if (delta) onDelta(delta);
          } catch (_e) {}
        }
        return pump();
      });
    }

    return pump();
  }

  function consumeJSON(res) {
    return res.json().then(function (obj) {
      if (!res.ok) {
        throw new Error(parseErrorBody(obj));
      }
      return extractMessageContent(obj);
    });
  }

  function isEventStream(res) {
    var ct = (res.headers.get("Content-Type") || "").toLowerCase();
    return ct.indexOf("text/event-stream") >= 0;
  }

  function handleResponse(res, onDelta) {
    if (!res.ok) {
      return res
        .json()
        .catch(function () {
          return {};
        })
        .then(function (obj) {
          throw new Error(parseErrorBody(obj) || "HTTP " + res.status);
        });
    }
    if (isEventStream(res)) {
      return consumeStream(res, onDelta);
    }
    return consumeJSON(res).then(function (text) {
      if (text && onDelta) onDelta(text);
    });
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Stream = {
    handleResponse: handleResponse,
    isEventStream: isEventStream
  };
})();
