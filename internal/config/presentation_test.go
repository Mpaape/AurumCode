package config

import "testing"

func TestReviewPresentationCollapseValidates(t *testing.T) {
	cfg := &Config{Review: ReviewConfig{Presentation: ReviewPresentationConfig{Collapse: []string{"Info", " warning "}}}}
	got, err := cfg.ReviewPresentationCollapse()
	if err != nil || !got["info"] || !got["warning"] || len(got) != 2 {
		t.Fatalf("collapse = %v, %v", got, err)
	}
	bad := &Config{Review: ReviewConfig{Presentation: ReviewPresentationConfig{Collapse: []string{"tudo"}}}}
	if _, err := bad.ReviewPresentationCollapse(); err == nil {
		t.Fatal("an unknown severity was accepted")
	}
	if got, err := (&Config{}).ReviewPresentationCollapse(); err != nil || got != nil {
		t.Fatalf("absent preference = %v, %v", got, err)
	}
}
