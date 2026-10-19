package collection

import "testing"

func TestPiInspectionCoverageProvenance(t *testing.T) {
	for _, scenario := range []string{
		"custom", "project", "native-known", "missing-reference", "reference-id", "reference-source", "unlisted-reference",
		"missing-root", "root-path", "root-context", "root-origin", "missing-request", "request-origin", "reference-context",
		"provenance", "native-fact", "native-unknown-permission", "native-unknown-participation",
	} {
		t.Run(scenario, func(t *testing.T) {
			ref := Reference{ID: "ref", Root: "root", Context: ContextCustom, Source: "source"}
			ra := ReferenceAvailability{Reference: ref.ID, Participation: ParticipationInspection, Permission: PermissionExplanation{State: PermissionUnknown}, Provenance: Provenance{Kind: "custom"}}
			source := Source{ID: ref.Source, References: []ReferenceID{ref.ID}}
			s := &Snapshot{
				references: map[ReferenceID]Reference{ref.ID: ref},
				request:    Request{Roots: []DiscoveryRoot{{ID: ref.Root, Path: "/fixture", Context: ContextCustom}}},
				scope:      Scope{Roots: []RootReport{{ID: ref.Root, Requested: "/fixture", Context: ContextCustom}}},
				piControls: &piInputs{facts: map[RootID]piRootFact{}},
			}
			switch scenario {
			case "project":
				ref.Context = ContextProject
				s.references[ref.ID] = ref
				s.request.Roots[0].Context = ContextProject
				s.scope.Roots[0].Context = ContextProject
			case "native-known", "native-unknown-permission", "native-unknown-participation":
				s.piControls.facts[ref.Root] = piRootFact{}
				ra.Participation = ParticipationEligible
				ra.Permission.State = PermissionAllowed
				if scenario == "native-unknown-permission" {
					ra.Permission.State = PermissionUnknown
				}
				if scenario == "native-unknown-participation" {
					ra.Participation = ParticipationUnknown
				}
			case "missing-reference":
				delete(s.references, ref.ID)
			case "reference-id":
				ref.ID = "other"
				s.references["ref"] = ref
			case "reference-source":
				ref.Source = "other"
				s.references[ref.ID] = ref
			case "unlisted-reference":
				source.References = nil
			case "missing-root":
				s.scope.Roots = nil
			case "root-path":
				s.scope.Roots[0].Requested = "/other"
			case "root-context":
				s.scope.Roots[0].Context = ContextGlobal
			case "root-origin":
				s.scope.Roots[0].Origin = "pi-auto-global-pi"
			case "missing-request":
				s.request.Roots = nil
			case "request-origin":
				s.request.Roots[0].Origin = "pi-configured-global"
			case "reference-context":
				ref.Context = ContextGlobal
				s.references[ref.ID] = ref
			case "provenance":
				ra.Provenance.Kind = "global"
			case "native-fact":
				s.piControls.facts[ref.Root] = piRootFact{}
			}
			source.Availability.References = []ReferenceAvailability{ra}
			err := piProveControlCoverage(s, source)
			want := scenario == "custom" || scenario == "project" || scenario == "native-known"
			if (err == nil) != want {
				t.Fatalf("coverage proof error = %v, want allowed=%t", err, want)
			}
		})
	}
}
