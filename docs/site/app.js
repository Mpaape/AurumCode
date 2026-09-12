"use strict";
const form = document.querySelector("#preferences");
function updateConfig() {
  const fields = [];
  const language = form.elements.language.value;
  const publication = form.elements.publication.value;
  if (language !== "en-US") fields.push("  language: " + language);
  if (publication !== "comments") fields.push("  publication: " + publication);
  if (form.elements.inline.checked) fields.push("  inline_comments: true");
  document.querySelector("#config-code").textContent = fields.length
    ? "review:\n" + fields.join("\n")
    : "# Defaults selecionados. Não é necessário criar este arquivo.";
  document.querySelector("#config-hint").textContent = publication === "review"
    ? "O review formal pode solicitar mudanças quando há achados bloqueantes."
    : "O parecer será publicado na conversa, sem um voto formal de aprovação.";
}
form.addEventListener("change", updateConfig);
updateConfig();
let feedbackTimer;
function feedback(message) {
  document.querySelector("#copy-status").textContent = message;
  clearTimeout(feedbackTimer);
  feedbackTimer = setTimeout(() => { document.querySelector("#copy-status").textContent = ""; }, 4500);
}
document.querySelectorAll("[data-copy]").forEach(button => {
  button.addEventListener("click", async () => {
    const code = document.getElementById(button.dataset.copy);
    try {
      await navigator.clipboard.writeText(code.textContent);
      feedback("Copiado. Cole no arquivo indicado.");
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
fetch("workflow.yml").then(response => {
  if (!response.ok) throw new Error("HTTP " + response.status);
  return response.text();
}).then(text => { document.querySelector("#workflow-code").textContent = text; })
  .catch(() => {
    document.querySelector("#workflow-code").textContent = "Não foi possível carregar o exemplo. Use o link Baixar acima.";
    document.querySelector('[data-copy="workflow-code"]').disabled = true;
  });
const links = [...document.querySelectorAll(".sidebar nav a")];
const observer = new IntersectionObserver(entries => {
  for (const entry of entries) {
    if (!entry.isIntersecting) continue;
    for (const link of links) {
      if (link.hash === "#" + entry.target.id) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    }
  }
}, { rootMargin: "-12% 0px -65% 0px" });
document.querySelectorAll("main section").forEach(section => observer.observe(section));
