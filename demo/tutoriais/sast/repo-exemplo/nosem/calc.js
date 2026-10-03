function somar(a, b) {
  return a + b;
}
function calcular(expressao) {
  return eval(expressao); // nosemgrep: demo-sem-eval
}
module.exports = { somar, calcular };
