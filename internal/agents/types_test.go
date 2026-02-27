package agents

import "testing"

func TestValidCapability(t *testing.T) {
	tests := []struct {
		cap  Capability
		want bool
	}{
		{CapScout, true},
		{CapBuilder, true},
		{CapReviewer, true},
		{CapLead, true},
		{CapMerger, true},
		{CapCoordinator, true},
		{CapSupervisor, true},
		{CapMonitor, true},
		{Capability("invalid"), false},
		{Capability(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			got := ValidCapability(tt.cap)
			if got != tt.want {
				t.Errorf("ValidCapability(%q) = %v, want %v", tt.cap, got, tt.want)
			}
		})
	}
}

func TestAllCapabilities(t *testing.T) {
	caps := AllCapabilities()
	if len(caps) != 8 {
		t.Errorf("AllCapabilities() returned %d items, want 8", len(caps))
	}
}

func TestValidSessionState(t *testing.T) {
	tests := []struct {
		state SessionState
		want  bool
	}{
		{StateBooting, true},
		{StateWorking, true},
		{StateCompleted, true},
		{StateStalled, true},
		{StateZombie, true},
		{SessionState("running"), false},
		{SessionState(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := ValidSessionState(tt.state)
			if got != tt.want {
				t.Errorf("ValidSessionState(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestAllSessionStates(t *testing.T) {
	states := AllSessionStates()
	if len(states) != 5 {
		t.Errorf("AllSessionStates() returned %d items, want 5", len(states))
	}
}
