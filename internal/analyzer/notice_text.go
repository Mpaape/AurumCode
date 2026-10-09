package analyzer

import "github.com/Mpaape/AurumCode/internal/i18n"

// noticeTextKeys maps a skipped file's Reason to its catalog text. A Reason
// without one (too large, no patch) keeps its Message, which carries the
// measured detail.
var noticeTextKeys = map[string]string{
	NoticeReasonBinary:    "terminal.notice_binary",
	noticeReasonGenerated: "terminal.notice_generated",
}

// Text is the notice as the review's language shows it: the catalog's text
// for a binary or generated file (in English, the bytes of Message), the
// Message itself otherwise.
func (n DiffNotice) Text(language string) string {
	if key, ok := noticeTextKeys[n.Reason]; ok {
		return i18n.Format(language, key, n.Path)
	}
	return n.Message
}
