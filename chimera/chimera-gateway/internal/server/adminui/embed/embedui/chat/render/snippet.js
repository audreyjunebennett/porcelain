/**
 * Snippet rendering: infer language from source path and highlight code or markdown.
 */
(function () {
  "use strict";

  function escapeHtml(s) {
    if (globalThis.ChimeraUI && ChimeraUI.escapeHtml) return ChimeraUI.escapeHtml(s);
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  var EXT_LANG = {
    ".go": "go",
    ".js": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
    ".jsx": "javascript",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".py": "python",
    ".pyw": "python",
    ".json": "json",
    ".yaml": "yaml",
    ".yml": "yaml",
    ".md": "markdown",
    ".markdown": "markdown",
    ".sql": "sql",
    ".sh": "shell",
    ".bash": "shell",
    ".zsh": "shell",
    ".css": "css",
    ".html": "html",
    ".htm": "html",
    ".rs": "rust",
    ".java": "java",
    ".xml": "xml",
    ".toml": "toml"
  };

  function inferLanguage(source, hint) {
    if (hint) return String(hint).trim().toLowerCase();
    var s = String(source || "").trim().toLowerCase();
    var dot = s.lastIndexOf(".");
    if (dot < 0) return "";
    return EXT_LANG[s.slice(dot)] || "";
  }

  function wrapRe(code, re, cls) {
    return code.replace(re, function (m) {
      return '<span class="chat-hl-' + cls + '">' + m + "</span>";
    });
  }

  var KEYWORDS = {
    go: /\b(?:package|import|func|var|const|type|struct|interface|map|chan|go|defer|return|if|else|for|range|switch|case|default|break|continue|select|fallthrough|nil|true|false|make|new|len|cap|append)\b/g,
    javascript: /\b(?:const|let|var|function|return|if|else|for|while|switch|case|break|continue|class|extends|import|export|from|async|await|try|catch|finally|throw|new|typeof|null|undefined|true|false)\b/g,
    typescript: /\b(?:const|let|var|function|return|if|else|for|while|switch|case|break|continue|class|extends|import|export|from|async|await|try|catch|finally|throw|new|typeof|null|undefined|true|false|interface|type|enum|implements|public|private|protected|readonly)\b/g,
    python: /\b(?:def|class|return|if|elif|else|for|while|break|continue|import|from|as|with|try|except|finally|raise|pass|lambda|yield|True|False|None|and|or|not|in|is|async|await)\b/g,
    rust: /\b(?:fn|let|mut|const|struct|enum|impl|trait|pub|use|mod|return|if|else|match|for|while|loop|break|continue|true|false|Self|self|async|await|where|type|move|ref|static|unsafe|extern|crate|super)\b/g,
    java: /\b(?:class|interface|enum|public|private|protected|static|final|void|return|if|else|for|while|switch|case|break|continue|new|import|package|extends|implements|throws|try|catch|finally|throw|true|false|null|this|super)\b/g,
    sql: /\b(?:SELECT|FROM|WHERE|JOIN|LEFT|RIGHT|INNER|OUTER|ON|GROUP|BY|ORDER|HAVING|LIMIT|OFFSET|INSERT|INTO|VALUES|UPDATE|SET|DELETE|CREATE|TABLE|INDEX|AND|OR|NOT|NULL|AS|DISTINCT|UNION|ALL|EXISTS|CASE|WHEN|THEN|ELSE|END)\b/g,
    shell: /\b(?:if|then|else|fi|for|do|done|case|esac|function|return|export|local|while|until|in|echo|exit|set|unset)\b/g
  };

  function highlightGeneric(code, lang) {
    var out = escapeHtml(code);
    out = wrapRe(out, /"(?:\\.|[^"\\])*"/g, "str");
    out = wrapRe(out, /'(?:\\.|[^'\\])*'/g, "str");
    out = wrapRe(out, /\/\/[^\n]*/g, "com");
    out = wrapRe(out, /#[^\n]*/g, "com");
    out = wrapRe(out, /--[^\n]*/g, "com");
    if (lang === "html" || lang === "xml") {
      out = wrapRe(out, /&lt;\/?[\w:-]+(?:\s+[\w:-]+(?:="[^"]*")?)*\s*\/?&gt;/g, "tag");
    }
    var kw = KEYWORDS[lang];
    if (kw) out = wrapRe(out, kw, "kw");
    if (lang === "json") {
      out = wrapRe(out, /"(?:\\.|[^"\\])*"(?=\s*:)/g, "key");
    }
    return out;
  }

  function closeSnippetHtml(html) {
    var md =
      globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Markdown &&
      typeof ChimeraChat.Render.Markdown.closeOpenHtmlTags === "function"
        ? ChimeraChat.Render.Markdown
        : null;
    return md ? md.closeOpenHtmlTags(html) : html;
  }

  function renderMarkdownSnippet(text) {
    var md =
      globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Markdown &&
      typeof ChimeraChat.Render.Markdown.render === "function"
        ? ChimeraChat.Render.Markdown
        : null;
    if (md) {
      var html =
        typeof md.renderSafe === "function"
          ? md.renderSafe(text)
          : typeof md.renderPartial === "function"
            ? md.renderPartial(text)
            : closeSnippetHtml(md.render(text));
      return '<div class="chat-embed-item__snippet chat-embed-item__snippet--md">' + html + "</div>";
    }
    return '<pre class="chat-embed-item__snippet chat-embed-item__snippet--code"><code>' + escapeHtml(text) + "</code></pre>";
  }

  function renderCodeSnippet(text, lang) {
    var label = lang ? ' data-lang="' + escapeHtml(lang) + '"' : "";
    return (
      '<pre class="chat-embed-item__snippet chat-embed-item__snippet--code"><code class="chat-hl"' +
      label +
      ">" +
      closeSnippetHtml(highlightGeneric(text, lang)) +
      "</code></pre>"
    );
  }

  function renderPlainSnippet(text) {
    return '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' + escapeHtml(text) + "</code></pre>";
  }

  function formatLineRangeLabel(startLine, endLine) {
    var s = Number(startLine);
    var e = Number(endLine);
    if (!s || s < 1 || isNaN(s)) return "";
    if (!e || e < s || isNaN(e)) e = s;
    return s === e ? "L" + s : "L" + s + "\u2013" + e;
  }

  function renderGutterSnippet(text, lang, lineMeta) {
    text = text == null ? "" : String(text);
    var start = Number(lineMeta && lineMeta.start_line != null ? lineMeta.start_line : 0);
    var end = Number(lineMeta && lineMeta.end_line != null ? lineMeta.end_line : 0);
    var mid = !!(lineMeta && (lineMeta.starts_mid_line === true || lineMeta.starts_mid_line === "true"));
    if (!start || start < 1 || isNaN(start)) {
      if (lang === "markdown") return renderMarkdownSnippet(text);
      if (lang) return renderCodeSnippet(text, lang);
      return renderPlainSnippet(text);
    }
    if (!end || end < start || isNaN(end)) end = start;
    var lines = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
    var nums = [];
    var rows = [];
    var li;
    for (li = 0; li < lines.length; li++) {
      var lineNo = start + li;
      if (lineNo > end) break;
      var rowText = lines[li];
      if (li === 0 && mid) rowText = "\u2026" + rowText;
      nums.push('<span class="chat-snippet-gutter__ln">' + String(lineNo) + "</span>");
      var inner = lang ? highlightGeneric(rowText, lang) : escapeHtml(rowText);
      rows.push('<span class="chat-snippet-gutter__row">' + inner + "</span>");
    }
    var label = lang ? ' data-lang="' + escapeHtml(lang) + '"' : "";
    return (
      '<pre class="chat-embed-item__snippet chat-embed-item__snippet--code chat-embed-item__snippet--gutter">' +
      '<code class="chat-snippet-gutter"' +
      label +
      ">" +
      '<span class="chat-snippet-gutter__nums" aria-hidden="true">' +
      nums.join("") +
      "</span>" +
      '<span class="chat-snippet-gutter__code">' +
      rows.join("") +
      "</span></code></pre>"
    );
  }

  function renderSnippet(source, text, languageHint, lineMeta) {
    text = text == null ? "" : String(text);
    var lang = inferLanguage(source, languageHint);
    if (lineMeta && Number(lineMeta.start_line) > 0) {
      return renderGutterSnippet(text, lang, lineMeta);
    }
    if (lang === "markdown") return renderMarkdownSnippet(text);
    if (lang) return renderCodeSnippet(text, lang);
    return renderPlainSnippet(text);
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Render = globalThis.ChimeraChat.Render || {};
  globalThis.ChimeraChat.Render.Snippet = {
    inferLanguage: inferLanguage,
    formatLineRangeLabel: formatLineRangeLabel,
    render: renderSnippet
  };
})();
