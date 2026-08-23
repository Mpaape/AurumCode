"use strict";

// Help text that teaches the user how to export their own key. This is
// documentation, not a credential -- the value is the literal placeholder
// word the project's own setup docs use.
const HELP_TEXT = "Run this first:\n  export GEMINI_API_KEY=sua-chave\n";

// Test fixtures below assign literal, synthetic values so tests can
// assert on redaction behavior. Nothing here is a real credential.
process.env.OPENAI_API_KEY = 'ttm-smoke-key';
process.env.OPENAI_API_KEY = 'sk-body-ULTRA-SECRET';

// Query built from constants only; the actual values are bound through
// '?' placeholders elsewhere via params.push(...), never concatenated in.
function rollupQuery(where) {
  return 'SELECT * FROM daily_rollups' + (where.length ? ' WHERE ' + where.join(' AND ') : '');
}

// AC-002: a real secret, a real SQL variable concat, and a real shell
// variable concat -- all three must still be found.
const apiKey = "Xk29LmQ8vTz41BhWn7Yc9";

function userQuery(userId) {
  return 'SELECT * FROM users WHERE id = ' + userId;
}

const { exec } = require("child_process");
function pingHost(host) {
  exec("ping -c 1 " + host);
}
