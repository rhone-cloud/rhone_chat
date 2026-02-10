function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

const languageKeywords = {
  go: ["func", "package", "import", "return", "if", "else", "for", "range", "switch", "case", "type", "struct", "interface", "var", "const", "go", "defer", "select", "map", "chan"],
  js: ["function", "const", "let", "var", "return", "if", "else", "for", "while", "switch", "case", "class", "new", "try", "catch", "async", "await", "import", "export"],
  ts: ["function", "const", "let", "var", "return", "if", "else", "for", "while", "switch", "case", "class", "new", "try", "catch", "async", "await", "import", "export", "interface", "type", "extends", "implements"],
  bash: ["if", "then", "else", "fi", "for", "in", "do", "done", "case", "esac", "while", "function", "echo", "export"],
  json: [],
  yaml: [],
  python: ["def", "class", "return", "if", "elif", "else", "for", "while", "import", "from", "as", "try", "except", "finally", "with", "lambda", "yield", "async", "await"],
};

function detectLanguage(rawLanguage) {
  const lang = String(rawLanguage || "").toLowerCase().trim();
  if (lang === "javascript") return "js";
  if (lang === "typescript") return "ts";
  if (lang === "sh" || lang === "shell" || lang === "zsh") return "bash";
  if (lang === "py") return "python";
  if (lang === "golang") return "go";
  if (lang === "yml") return "yaml";
  if (lang in languageKeywords) return lang;
  return "";
}

function wrapWithLanguageKeywords(escapedCode, lang) {
  const keywords = languageKeywords[lang];
  if (!keywords || keywords.length === 0) {
    return escapedCode;
  }
  const pattern = new RegExp(`\\b(${keywords.map(escapeRegExp).join("|")})\\b`, "g");
  return escapedCode.replace(pattern, '<span class="md-code-keyword">$1</span>');
}

function highlightJSON(rawCode) {
  let escaped = escapeHTML(rawCode);
  escaped = escaped.replace(/"(\\.|[^"\\])*"(?=\s*:)/g, '<span class="md-code-key">$&</span>');
  escaped = escaped.replace(/"(\\.|[^"\\])*"/g, '<span class="md-code-string">$&</span>');
  escaped = escaped.replace(/\b(true|false|null)\b/g, '<span class="md-code-keyword">$1</span>');
  escaped = escaped.replace(/\b-?\d+(\.\d+)?([eE][+-]?\d+)?\b/g, '<span class="md-code-number">$&</span>');
  return escaped;
}

function highlightYAML(rawCode) {
  let escaped = escapeHTML(rawCode);
  escaped = escaped.replace(/^(\s*[\w-]+)(\s*:)/gm, '<span class="md-code-key">$1</span>$2');
  escaped = escaped.replace(/"(\\.|[^"\\])*"|'([^'\\]|\\.)*'/g, '<span class="md-code-string">$&</span>');
  escaped = escaped.replace(/\b(true|false|null|yes|no)\b/gi, '<span class="md-code-keyword">$&</span>');
  escaped = escaped.replace(/\b-?\d+(\.\d+)?\b/g, '<span class="md-code-number">$&</span>');
  return escaped;
}

function highlightGeneric(rawCode, lang) {
  let escaped = escapeHTML(rawCode);
  escaped = escaped.replace(/("(\\.|[^"\\])*"|'([^'\\]|\\.)*')/g, '<span class="md-code-string">$1</span>');
  escaped = escaped.replace(/\b-?\d+(\.\d+)?\b/g, '<span class="md-code-number">$&</span>');
  escaped = escaped.replace(/\/\/[^\n]*/g, '<span class="md-code-comment">$&</span>');
  escaped = escaped.replace(/#[^\n]*/g, '<span class="md-code-comment">$&</span>');
  return wrapWithLanguageKeywords(escaped, lang);
}

function highlightCode(rawCode, rawLanguage) {
  const lang = detectLanguage(rawLanguage);
  if (lang === "json") {
    return highlightJSON(rawCode);
  }
  if (lang === "yaml") {
    return highlightYAML(rawCode);
  }
  return highlightGeneric(rawCode, lang);
}

function applySyntaxHighlighting(root) {
  const nodes = root.querySelectorAll("pre code");
  for (const node of nodes) {
    const className = node.className || "";
    const match = /language-([a-z0-9_-]+)/i.exec(className);
    const lang = match ? match[1] : "";
    const raw = node.textContent || "";
    node.innerHTML = highlightCode(raw, lang);
  }
}

function sanitizeURL(raw) {
  if (!raw) {
    return "";
  }
  const trimmed = String(raw).trim();
  if (trimmed.startsWith("#") || trimmed.startsWith("/")) {
    return trimmed;
  }
  try {
    const parsed = new URL(trimmed, window.location.origin);
    if (parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "mailto:" || parsed.protocol === "tel:") {
      return parsed.href;
    }
  } catch (_) {
    return "";
  }
  return "";
}

function renderInline(input) {
  const codeTokens = [];
  let text = escapeHTML(input);

  text = text.replace(/`([^`]+)`/g, (_, code) => {
    const token = `@@CODE_${codeTokens.length}@@`;
    codeTokens.push(`<code>${escapeHTML(code)}</code>`);
    return token;
  });

  text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, url) => {
    const safeURL = sanitizeURL(url);
    if (!safeURL) {
      return escapeHTML(label);
    }
    return `<a href="${escapeHTML(safeURL)}" target="_blank" rel="noopener noreferrer">${escapeHTML(label)}</a>`;
  });

  text = text.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  text = text.replace(/(^|[^*])\*([^*]+)\*/g, "$1<em>$2</em>");

  for (let index = 0; index < codeTokens.length; index += 1) {
    text = text.replace(`@@CODE_${index}@@`, codeTokens[index]);
  }

  return text;
}

function renderMarkdown(markdown) {
  const lines = String(markdown || "").replace(/\r\n/g, "\n").split("\n");
  const out = [];

  let paragraph = [];
  let listItems = [];
  let listType = "";
  let inFence = false;
  let fenceLang = "";
  let fenceLines = [];

  function flushParagraph() {
    if (paragraph.length === 0) {
      return;
    }
    const joined = paragraph.join(" ").trim();
    if (joined) {
      out.push(`<p>${renderInline(joined)}</p>`);
    }
    paragraph = [];
  }

  function flushList() {
    if (listItems.length === 0) {
      return;
    }
    const tag = listType === "ol" ? "ol" : "ul";
    out.push(`<${tag}>${listItems.map((item) => `<li>${renderInline(item)}</li>`).join("")}</${tag}>`);
    listItems = [];
    listType = "";
  }

  function flushFence() {
    if (!inFence) {
      return;
    }
    const langClass = fenceLang ? ` class="language-${escapeHTML(fenceLang)}"` : "";
    out.push(`<pre><code${langClass}>${escapeHTML(fenceLines.join("\n"))}</code></pre>`);
    inFence = false;
    fenceLang = "";
    fenceLines = [];
  }

  for (const rawLine of lines) {
    const line = rawLine ?? "";
    const trimmed = line.trim();

    if (trimmed.startsWith("```")) {
      if (inFence) {
        flushFence();
      } else {
        flushParagraph();
        flushList();
        inFence = true;
        fenceLang = trimmed.slice(3).trim();
      }
      continue;
    }

    if (inFence) {
      fenceLines.push(line);
      continue;
    }

    if (trimmed === "") {
      flushParagraph();
      flushList();
      continue;
    }

    const headingMatch = /^(#{1,6})\s+(.+)$/.exec(trimmed);
    if (headingMatch) {
      flushParagraph();
      flushList();
      const level = headingMatch[1].length;
      out.push(`<h${level}>${renderInline(headingMatch[2])}</h${level}>`);
      continue;
    }

    const quoteMatch = /^>\s?(.*)$/.exec(trimmed);
    if (quoteMatch) {
      flushParagraph();
      flushList();
      out.push(`<blockquote>${renderInline(quoteMatch[1])}</blockquote>`);
      continue;
    }

    const ulMatch = /^[-*+]\s+(.+)$/.exec(trimmed);
    if (ulMatch) {
      flushParagraph();
      if (listType !== "ul") {
        flushList();
        listType = "ul";
      }
      listItems.push(ulMatch[1]);
      continue;
    }

    const olMatch = /^\d+\.\s+(.+)$/.exec(trimmed);
    if (olMatch) {
      flushParagraph();
      if (listType !== "ol") {
        flushList();
        listType = "ol";
      }
      listItems.push(olMatch[1]);
      continue;
    }

    flushList();
    paragraph.push(trimmed);
  }

  flushFence();
  flushParagraph();
  flushList();

  return out.join("");
}

function applyTheme(el, theme) {
  const mode = theme === "light" ? "light" : "dark";
  el.dataset.mdTheme = mode;
}

function render(el, props) {
  const markdown = typeof props?.markdown === "string" ? props.markdown : "";
  applyTheme(el, props?.theme);
  el.classList.add("md-renderer");
  el.innerHTML = renderMarkdown(markdown);
  applySyntaxHighlighting(el);
}

export function mount(el, props) {
  render(el, props);
  return {
    update(nextProps) {
      render(el, nextProps);
    },
    destroy() {
      el.innerHTML = "";
    },
  };
}
