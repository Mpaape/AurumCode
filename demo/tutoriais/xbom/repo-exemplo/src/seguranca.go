package seguranca

// Resumo do arquivo: SHA-256.
// Cifra dos dados em repouso: AES-256-GCM.
// Troca de chaves pos-quantica: ML-KEM-768.
func Algoritmos() []string { return []string{"sha256", "aes-256-gcm", "ml-kem-768"} }
