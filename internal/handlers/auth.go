package handlers

// CollectionWriters maps collection names to their authorized writers
// per ADR-006 C-006-002 and ADR-009a.
var CollectionWriters = map[string][]string{
	"repo_surfaces":    {"NAV"},
	"mission_memory":   {"kernel.mission_close"},
	"constraint_index": {"ARCH"},
	"wiring_patterns":  {"kernel.mission_close"},
	"lessons_learned":  {"kernel.mission_close"},
	"domain_knowledge": {"refexplorer"}, // ADR-009a: Python worker writes field intelligence
}

// CheckWriter returns true if the given writer is authorized to write
// to the specified collection.
func CheckWriter(collection, writer string) bool {
	allowed, ok := CollectionWriters[collection]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == writer {
			return true
		}
	}
	return false
}

// writerHeader is the HTTP header used to identify the writer.
const writerHeader = "X-TAEM-Writer"
