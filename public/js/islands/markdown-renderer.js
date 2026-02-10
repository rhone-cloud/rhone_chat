function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
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
