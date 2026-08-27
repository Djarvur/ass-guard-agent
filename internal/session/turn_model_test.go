package session

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// TestSetTurnModel (16-05, ACP-08 live apply): SetTurnModel stamps the
// session's profile copy so the NEXT provider request is shaped with the new
// model (the Shaper reads Model from the profile at every Stream).
func TestSetTurnModel(t *testing.T) {
	t.Parallel()

	s := &Session{Profile: profile.Profile{Model: "model-a"}}

	s.SetTurnModel("model-b")

	if s.Profile.Model != "model-b" {
		t.Errorf("Profile.Model = %q; want model-b after SetTurnModel (next request's shape)", s.Profile.Model)
	}
}
