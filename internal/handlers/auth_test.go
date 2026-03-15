package handlers

import "testing"

func TestCheckWriter(t *testing.T) {
	tests := []struct {
		name       string
		collection string
		writer     string
		want       bool
	}{
		{"NAV can write repo_surfaces", "repo_surfaces", "NAV", true},
		{"kernel.mission_close can write mission_memory", "mission_memory", "kernel.mission_close", true},
		{"ARCH can write constraint_index", "constraint_index", "ARCH", true},
		{"kernel.mission_close can write wiring_patterns", "wiring_patterns", "kernel.mission_close", true},
		{"kernel.mission_close can write lessons_learned", "lessons_learned", "kernel.mission_close", true},

		// Unauthorized
		{"ARCH cannot write repo_surfaces", "repo_surfaces", "ARCH", false},
		{"NAV cannot write mission_memory", "mission_memory", "NAV", false},
		{"NAV cannot write constraint_index", "constraint_index", "NAV", false},
		{"empty writer is unauthorized", "repo_surfaces", "", false},
		{"unknown collection is unauthorized", "unknown", "NAV", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckWriter(tt.collection, tt.writer)
			if got != tt.want {
				t.Errorf("CheckWriter(%q, %q) = %v, want %v", tt.collection, tt.writer, got, tt.want)
			}
		})
	}
}
