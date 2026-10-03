function somar(a, b) {
  return a + b;
}
function calcular(expressao) {
  return eval(expressao);
}
function subtrair(a, b) {
  if (typeof a !== "number" || typeof b !== "number") {
    throw new TypeError("subtrair: argumentos numericos");
  }
  return a - b;
}
function multiplicar(a, b) {
  if (typeof a !== "number" || typeof b !== "number") {
    throw new TypeError("multiplicar: argumentos numericos");
  }
  return a * b;
}
function dividir(a, b) {
  if (typeof a !== "number" || typeof b !== "number") {
    throw new TypeError("dividir: argumentos numericos");
  }
  return a / b;
}
function resto(a, b) {
  if (typeof a !== "number" || typeof b !== "number") {
    throw new TypeError("resto: argumentos numericos");
  }
  return a % b;
}
const operacoes = {
  "+": somar,
  "-": subtrair,
  "*": multiplicar,
  "/": dividir,
  "%": resto,
};
function aplicar(op, a, b) {
  const f = operacoes[op];
  if (!f) {
    throw new Error("operacao desconhecida: " + op);
  }
  return f(a, b);
}
module.exports = { somar, subtrair, multiplicar, dividir, resto, aplicar, calcular };
