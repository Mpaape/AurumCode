const _ = require("lodash");

// Avalia a expressao digitada pelo usuario (planted: SAST deve reprovar).
function calcular(expressao) {
  return eval(expressao);
}

module.exports = { calcular, somar: (a, b) => _.add(a, b) };
