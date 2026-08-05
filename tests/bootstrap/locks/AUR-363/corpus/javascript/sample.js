/**
 * Bounded, non-sensitive fixture corpus for the AUR-363 JavaScript adapter
 * lock. It exists only to be digested, never parsed.
 *
 * @param {string} name
 * @returns {string}
 */
function greet(name) {
  return `hello, ${name}`;
}

module.exports = { greet };
