package clienthost

import "testing"

func TestPlayBlockedMessageMissingBizHawk(t *testing.T) {
	snap := DependenciesSnapshot{
		PlayBlocked: true,
		Items: []DependencyItem{{
			ID:     DependencyBizHawk,
			Status: "missing",
		}},
	}
	msg := PlayBlockedMessage(snap)
	if msg == "" {
		t.Fatal("expected message")
	}
}

func TestPlayBlockedMessageOutdated(t *testing.T) {
	snap := DependenciesSnapshot{
		PlayBlocked: true,
		Items: []DependencyItem{{
			ID:     DependencyBizHawk,
			Status: "outdated",
			Detail: "v2.9",
		}},
	}
	if PlayBlockedMessage(snap) == "" {
		t.Fatal("expected message")
	}
}
