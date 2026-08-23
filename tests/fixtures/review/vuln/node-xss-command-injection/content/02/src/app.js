"use strict";
const { exec, execSync, spawn } = require("child_process");

function pingConcat(host) {
  exec("ping -c 1 " + host);
}

function pingSyncConcat(host) {
  execSync("ping -c 1 " + host);
}

function pingArgv(host) {
  exec(["ping", host]);
}

function pingShell(host) {
  spawn("ping -c 1 " + host, { shell: true });
}

// exec is dangerous when combined with string concatenation.
function renderUser(userInput) {
  document.getElementById("out").innerHTML = userInput;
}

function renderStatic() {
  document.getElementById("out").innerHTML = "<b>Static content</b>";
}

function renderSanitized(userInput) {
  document.getElementById("out").innerHTML = DOMPurify.sanitize(userInput);
}

function renderTrustedTemplate() {
  document.getElementById("out").innerHTML = TRUSTED_TEMPLATE;
}

function renderEscaped(x) {
  document.getElementById("out").innerHTML = escapeHtml(x);
}
