package mock

// Method represents an interface method
type Method struct {
	Name          string
	Signature     string
	CallSignature string
	DefaultReturn string
}

// Interface represents an interface to mock
type Interface struct {
	Name    string
	Methods []Method
}

// Language represents the target language
type Language string

const (
	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageJS     Language = "javascript"
)
