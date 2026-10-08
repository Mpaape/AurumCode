package main

import "fmt"

// Limites e a secao de limites da configuracao (nil = nao declarada).
type Limites struct {
	Maximo int
}

// Validar recusa um maximo negativo; sem secao, nao ha o que validar.
func (l *Limites) Validar() error {
	if l == nil {
		return nil
	}
	if l.Maximo < 0 {
		return fmt.Errorf("maximo negativo: %d", l.Maximo)
	}
	return nil
}

// carregar valida a secao declarada, se houver.
func carregar(l *Limites) error {
	return l.Validar()
}
