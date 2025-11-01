package cost

import (
	"testing"
)

func TestCostTrackerAllow(t *testing.T) {
	prices := map[string]PriceMap{
		"gpt-4": {
			InputPer1K:  0.03,
			OutputPer1K: 0.06,
		},
	}
	
	tracker := NewTracker(10.0, 100.0, prices)
	
	// Should allow within budget
	if !tracker.Allow(1000, 500, "gpt-4") {
		t.Error("Expected to allow request within budget")
	}
	
	// Should block when over budget
	if tracker.Allow(10000000, 10000000, "gpt-4") {
		t.Error("Expected to block request over budget")
	}
}

func TestCostTrackerSpend(t *testing.T) {
	prices := map[string]PriceMap{
		"gpt-4": {
			InputPer1K:  0.03,
			OutputPer1K: 0.06,
		},
	}
	
	tracker := NewTracker(10.0, 100.0, prices)
	
	// Spend some tokens
	err := tracker.Spend(1000, 500, "gpt-4")
	if err != nil {
		t.Fatalf("Spend failed: %v", err)
	}
	
	// Calculate expected cost: (1000/1000)*0.03 + (500/1000)*0.06 = 0.03 + 0.03 = 0.06
	remaining, _ := tracker.Remaining()
	if remaining > 9.94 && remaining < 9.94 {
		t.Errorf("Expected remaining around 9.94, got %f", remaining)
	}
}

func TestCostTrackerRemaining(t *testing.T) {
	prices := map[string]PriceMap{
		"gpt-4": {
			InputPer1K:  0.03,
			OutputPer1K: 0.06,
		},
	}
	
	tracker := NewTracker(10.0, 100.0, prices)
	
	perRun, daily := tracker.Remaining()
	
	if perRun != 10.0 {
		t.Errorf("Expected 10.0 per-run remaining, got %f", perRun)
	}
	
	if daily != 100.0 {
		t.Errorf("Expected 100.0 daily remaining, got %f", daily)
	}
}

func TestCostTrackerDailyReset(t *testing.T) {
	prices := map[string]PriceMap{
		"gpt-4": {
			InputPer1K:  0.03,
			OutputPer1K: 0.06,
		},
	}
	
	tracker := NewTracker(10.0, 100.0, prices)
	
	// Spend up to daily budget
	err := tracker.Spend(10000, 10000, "gpt-4")
	if err != nil {
		t.Fatalf("Spend failed: %v", err)
	}
	
	_, daily := tracker.Remaining()
	if daily >= 100.0 {
		t.Errorf("Expected daily remaining to decrease after spend")
	}
	
	// Reset per-run should not affect daily
	tracker.ResetPerRun()
	_, daily2 := tracker.Remaining()
	
	if daily != daily2 {
		t.Errorf("Daily budget should not change after per-run reset")
	}
}

