"use strict";
const language = document.getElementById("language");
const publication = document.getElementById("publication");
const inline = document.getElementById("inline");
function updateConfig() {
  const lines = [];
  if (language.value !== "en-US") lines.push("  language: " + language.value);
  if (publication.value !== "comments") lines.push("  publication: " + publication.value);
  if (inline.checked) lines.push("  inline_comments: true");
  document.getElementById("config-code").textContent = lines.length
    ? "review:\n" + lines.join("\n")
    : "# Não é necessário criar .aurumcode.yaml para usar os padrões.";
}
if (language && publication && inline) {
  document.querySelector(".config-controls").hidden = false;
  [language, publication, inline].forEach(control => control.addEventListener("change", updateConfig));
  updateConfig();
}
// Progressive enhancement only: every snippet on this page is already static,
// inline HTML (see index.html's #workflow-code). Without this script the page
// is fully readable and copyable by hand; with it, a click copies the code
// block to the clipboard and shows a small confirmation toast.

let feedbackTimer;
function feedback(message) {
  const toast = document.querySelector("#copy-status");
  if (!toast) return;
  toast.textContent = message;
  clearTimeout(feedbackTimer);
  feedbackTimer = setTimeout(() => { toast.textContent = ""; }, 4500);
}

document.querySelectorAll("[data-copy]").forEach(button => {
  button.hidden = false;
  button.addEventListener("click", async () => {
    const code = document.getElementById(button.dataset.copy);
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code.textContent);
      feedback("Copiado.");
    } catch {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents(code);
      selection.removeAllRanges();
      selection.addRange(range);
      feedback("Texto selecionado. Use Ctrl+C ou ⌘C para copiar.");
    }
  });
});

// Highlight the current section in the top nav while scrolling. Cosmetic
// only: the nav links work as plain anchors with no script at all.
const links = [...document.querySelectorAll(".topnav a")];
if (links.length && "IntersectionObserver" in window) {
  const observer = new IntersectionObserver(entries => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      for (const link of links) {
        if (link.hash === "#" + entry.target.id) link.setAttribute("aria-current", "location");
        else link.removeAttribute("aria-current");
      }
    }
  }, { rootMargin: "-12% 0px -70% 0px" });
  document.querySelectorAll("main section[id]").forEach(section => observer.observe(section));
}
