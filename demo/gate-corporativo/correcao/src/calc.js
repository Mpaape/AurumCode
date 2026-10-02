const _ = require("lodash");

// Somente operacoes aritmeticas simples, sem avaliar codigo arbitrario.
const operacoes = { "+": _.add, "-": _.subtract, "*": _.multiply, "/": _.divide };

function calcular(a, operador, b) {
  const operacao = operacoes[operador];
  if (!operacao) {
    throw new Error("operador invalido");
  }
  return operacao(a, b);
}

module.exports = { calcular, somar: (a, b) => _.add(a, b) };
