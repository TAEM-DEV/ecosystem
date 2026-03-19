package collections

import (
	"fmt"
	"log"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

// Names lists all five ADR-006 Qdrant collections (1536-dim).
var Names = []string{
	"taem_repo_surfaces",
	"taem_mission_memory",
	"taem_constraint_index",
	"taem_wiring_patterns",
	"taem_lessons_learned",
}

// VectorSize is the embedding dimension for the original 5 collections
// (OpenAI Ada compatible).
const VectorSize = 1536

// DomainKnowledgeCollection is the ADR-009a collection for field intelligence.
// Uses 768-dim vectors from nomic-embed-text (C-009-008).
const DomainKnowledgeCollection = "domain_knowledge"

// DomainKnowledgeVectorSize is the embedding dimension for domain_knowledge
// (nomic-embed-text output dimension per ADR-009a C-009-008).
const DomainKnowledgeVectorSize = 768

// InitAll creates all Qdrant collections on startup. Safe to call
// on every startup — idempotent. If a collection already exists it is skipped.
// Creates the 5 ADR-006 collections (1536-dim) and the ADR-009a
// domain_knowledge collection (768-dim).
func InitAll(client *qdrant.Client) error {
	// Original 5 collections at 1536-dim
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

	// ADR-009a: domain_knowledge at 768-dim (nomic-embed-text)
	exists, err := client.CollectionExists(DomainKnowledgeCollection)
	if err != nil {
		return fmt.Errorf("checking collection %s: %w", DomainKnowledgeCollection, err)
	}
	if exists {
		log.Printf("collection %s already exists, skipping", DomainKnowledgeCollection)
	} else {
		if err := client.CreateCollection(DomainKnowledgeCollection, DomainKnowledgeVectorSize); err != nil {
			return fmt.Errorf("creating collection %s: %w", DomainKnowledgeCollection, err)
		}
		log.Printf("created collection %s (768-dim, nomic-embed-text)", DomainKnowledgeCollection)
	}

	return nil
}
