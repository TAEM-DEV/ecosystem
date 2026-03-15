package collections

import (
	"fmt"
	"log"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

// Names lists all five Qdrant collections per ADR-006 C-006-001.
var Names = []string{
	"taem_repo_surfaces",
	"taem_mission_memory",
	"taem_constraint_index",
	"taem_wiring_patterns",
	"taem_lessons_learned",
}

// VectorSize is the embedding dimension (OpenAI Ada compatible).
const VectorSize = 1536

// InitAll creates all 5 Qdrant collections on startup. Safe to call
// on every startup — idempotent. If a collection already exists it is skipped.
func InitAll(client *qdrant.Client) error {
	for _, name := range Names {
		exists, err := client.CollectionExists(name)
		if err != nil {
			return fmt.Errorf("checking collection %s: %w", name, err)
		}
		if exists {
			log.Printf("collection %s already exists, skipping", name)
			continue
		}
		if err := client.CreateCollection(name, VectorSize); err != nil {
			return fmt.Errorf("creating collection %s: %w", name, err)
		}
		log.Printf("created collection %s", name)
	}
	return nil
}
