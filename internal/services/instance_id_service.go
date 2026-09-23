package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"windshift/internal/repository"
)

// instanceIDSettingKey stores the install identity used by plugin licensing.
const instanceIDSettingKey = "instance_id"

// InstanceService provides the stable identity of this installation. The ID
// is generated once (64 bits of entropy, formatted XXXX-XXXX-XXXX-XXXX) and
// persisted, so it survives restarts and travels with the database.
type InstanceService struct {
	repo *repository.SystemSettingRepository

	mu   sync.Mutex
	id   string
	done bool
}

// NewInstanceService creates an InstanceService.
func NewInstanceService(repo *repository.SystemSettingRepository) *InstanceService {
	return &InstanceService{repo: repo}
}

// GetOrCreate returns this install's instance ID, generating and persisting it
// on first use. Concurrent callers block until the ID is settled.
func (s *InstanceService) GetOrCreate() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return s.id, nil
	}

	existing, ok, err := s.repo.GetValue(instanceIDSettingKey)
	if err != nil {
		return "", fmt.Errorf("load instance id: %w", err)
	}
	if ok && existing != "" {
		s.id = existing
		s.done = true
		return s.id, nil
	}

	generated, err := newInstanceID()
	if err != nil {
		return "", fmt.Errorf("generate instance id: %w", err)
	}
	if err := s.repo.Upsert(instanceIDSettingKey, generated, "string", "Stable identity of this installation (plugin licensing)", "system"); err != nil {
		return "", fmt.Errorf("store instance id: %w", err)
	}
	s.id = generated
	s.done = true
	return s.id, nil
}

// newInstanceID returns 64 bits of entropy as XXXX-XXXX-XXXX-XXXX. Short
// enough to read over the phone; long enough that regenerating IDs until one
// matches a stolen license stays infeasible.
func newInstanceID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	hexed := strings.ToUpper(hex.EncodeToString(raw))
	var groups []string
	for i := 0; i < len(hexed); i += 4 {
		groups = append(groups, hexed[i:i+4])
	}
	return strings.Join(groups, "-"), nil
}
