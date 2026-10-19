//go:build darwin

package collection

import (
	"context"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"testing"
)

func TestSpecALE52ReplacementReceiptUsesApprovedPolicy(t *testing.T) {
	snapshot, id, config := faultFixture(t, false)
	preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable})
	if preview.plan == nil {
		t.Fatal(preview.View())
	}
	out := preview.Apply(context.Background(), preview.View().Confirmation)
	if out.Persistence != "verified" {
		t.Fatal("same-origin baseline did not save", out)
	}
	actual, err := observeChangePath(config)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	// Model only the ORIGINAL observation in memory as a different process origin.
	// Never write, remove or spoof any filesystem provenance/security attribute.
	original := map[string][]byte{}
	for name, value := range preview.plan.destination.Config.Attributes {
		original[name] = append([]byte(nil), value...)
	}
	original["com.apple.provenance"] = append(append([]byte(nil), actual.Attributes["com.apple.provenance"]...), 0x7f)
	preview.plan.destination.Config.Attributes = original
	if !replacementMetadataMatches(preview.plan.destination.Config, actual) {
		t.Fatal("approved staged replacement comparator rejected modeled difference")
	}
	receipt := preview.plan.inspectResult(ChangeOutcome{State: "applied", WriteState: "committed", Persistence: "unverified"}, preview.plan.destination.Parent.Identity)
	afterBytes, err := os.ReadFile(config)
	if err != nil || string(beforeBytes) != string(afterBytes) {
		t.Fatal("inspection modified bytes", err)
	}
	t.Logf("approved replacement metadata match=true; persisted bytes exact=true; receipt persistence=%s achievement=%s state=%s", receipt.Persistence, receipt.Achievement, receipt.State)
	if receipt.Persistence != "verified" {
		t.Fatalf("approved provenance-only replacement was committed but receipt persistence=%s, want verified", receipt.Persistence)
	}
}

func TestALE52ResultInspectionKeepsExactMetadata(t *testing.T) {
	for _, scenario := range []string{"no-op-provenance", "no-op-ordinary-attribute", "committed-without-staging", "staged-provenance", "staged-ordinary-attribute", "staged-flags", "staged-links", "same-byte-replacement"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot, id, config := faultFixture(t, false)
			preview := snapshot.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable})
			if preview.plan == nil {
				t.Fatal(preview.View())
			}
			out := preview.Apply(context.Background(), preview.View().Confirmation)
			if out.ExitCode() != 0 {
				t.Fatal(out)
			}
			plan := preview.plan
			state := "committed"
			switch scenario {
			case "no-op-provenance", "no-op-ordinary-attribute":
				fresh, err := Build(context.Background(), snapshot.request)
				if err != nil {
					t.Fatal(err)
				}
				plan = fresh.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable}).plan
				if plan == nil {
					t.Fatal("no no-op plan")
				}
				state = "none"
				attributes := map[string][]byte{}
				for name, value := range plan.destination.Config.Attributes {
					attributes[name] = append([]byte(nil), value...)
				}
				key := "com.apple.provenance"
				if scenario == "no-op-ordinary-attribute" {
					key = "org.skillglass.fixture"
				}
				attributes[key] = append(attributes[key], 0x7f)
				plan.destination.Config.Attributes = attributes
			case "committed-without-staging":
				plan.stagedFact = nil
			case "staged-provenance", "staged-ordinary-attribute":
				staged := *plan.stagedFact
				staged.Attributes = map[string][]byte{}
				for name, value := range plan.stagedFact.Attributes {
					staged.Attributes[name] = append([]byte(nil), value...)
				}
				key := "com.apple.provenance"
				if scenario == "staged-ordinary-attribute" {
					key = "org.skillglass.fixture"
				}
				staged.Attributes[key] = append(staged.Attributes[key], 0x7f)
				plan.stagedFact = &staged
			case "staged-flags":
				plan.stagedFact.Flags = unix.UF_NODUMP
			case "staged-links":
				plan.stagedFact.Links++
			case "same-byte-replacement":
				raw, err := os.ReadFile(config)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(config+".external", raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(config+".external", config); err != nil {
					t.Fatal(err)
				}
			}
			receipt := plan.inspectResult(ChangeOutcome{State: "applied", WriteState: state, Persistence: "unverified"}, plan.destination.Parent.Identity)
			if receipt.Persistence == "verified" || plan.committedFact != nil {
				t.Fatal("unproven result certified", scenario, receipt)
			}
		})
	}
}

func TestALE52OrderedProgressionUsesVerifiedReplacement(t *testing.T) {
	snapshot, _, config := faultFixture(t, false)
	second := filepath.Join(snapshot.request.home, ".agents/skills/second/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(second), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("---\nname: second\ndescription: Inert fixture.\n---\nNo execution.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Build(context.Background(), snapshot.request)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.order) != 2 {
		t.Fatal(snapshot.order)
	}
	guard := &reviewGuard{facts: map[string]changeFact{}, failures: map[string]string{}}
	initial := snapshot.PreviewChange(context.Background(), ChangeIntent{snapshot.order[0], "codex", ControlGlobal, ControlDisable})
	if initial.plan == nil {
		t.Fatal(initial.View())
	}
	for _, fact := range initial.plan.dependencies {
		guard.facts[fact.Path] = fact
	}
	guard.facts[config] = initial.plan.destination.Config
	for _, id := range snapshot.order {
		fresh, err := Build(context.Background(), snapshot.request)
		if err != nil {
			t.Fatal(err)
		}
		preview := fresh.PreviewChange(context.Background(), ChangeIntent{id, "codex", ControlGlobal, ControlDisable})
		if preview.plan == nil {
			t.Fatal(preview.View())
		}
		plan := preview.plan
		plan.reviewCheck = guard.check
		plan.fault = func(stage string) error {
			if stage == "postread" {
				// Model the distinct original observation only after the real precommit
				// checks. The filesystem security tag is never changed by this fixture.
				attributes := map[string][]byte{}
				for name, value := range plan.destination.Config.Attributes {
					attributes[name] = append([]byte(nil), value...)
				}
				attributes["com.apple.provenance"] = append(attributes["com.apple.provenance"], 0x7f)
				plan.destination.Config.Attributes = attributes
			}
			return nil
		}
		result := preview.Apply(context.Background(), preview.View().Confirmation)
		if result.ExitCode() != 0 || result.WriteState != "committed" {
			t.Fatal("ordered effect stopped", result)
		}
		if err := guard.advance(plan, nil); err != nil {
			t.Fatal("verified replacement did not advance", err)
		}
	}
	actual, err := observeChangePath(config)
	if err != nil {
		t.Fatal(err)
	}
	adopted := guard.facts[config]
	if !sameChangeFacts(actual, adopted) {
		t.Fatal("guard did not adopt complete committed fact")
	}
	attributes := map[string][]byte{}
	for name, value := range adopted.Attributes {
		attributes[name] = append([]byte(nil), value...)
	}
	attributes["com.apple.provenance"] = append(attributes["com.apple.provenance"], 0x7f)
	adopted.Attributes = attributes
	guard.facts[config] = adopted
	if guard.check(context.Background()) == nil {
		t.Fatal("subsequent freshness ignored provenance")
	}
}
