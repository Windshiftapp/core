package operatorcontrols

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Controls holds an immutable policy and the instance's admission state.
type Controls struct {
	policy Policy
	mu     sync.Mutex
	active map[string]int
	store  BudgetStore
	now    func() time.Time
}

func New(policy Policy, stores ...BudgetStore) (*Controls, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	policy.Deny = slices.Clone(policy.Deny)
	if policy.Mail != nil {
		mail := *policy.Mail
		policy.Mail = &mail
	}
	if policy.AI != nil {
		ai := *policy.AI
		ai.AllowedModels = slices.Clone(ai.AllowedModels)
		ai.DeniedFeatures = slices.Clone(ai.DeniedFeatures)
		policy.AI = &ai
	}
	if policy.Imports != nil {
		imports := *policy.Imports
		policy.Imports = &imports
	}
	c := &Controls{policy: policy, active: make(map[string]int), now: time.Now}
	if len(stores) > 0 {
		c.store = stores[0]
	}
	return c, nil
}

func (c *Controls) Check(capability Capability) error {
	if c != nil && c.policy.Mail != nil {
		m := c.policy.Mail
		if capability == MailSend && (m.Mode == "disabled" || m.Suspended) {
			return &DeniedError{Capability: MailSend}
		}
		if capability == MailTransport && m.Mode == "managed" {
			return &DeniedError{Capability: MailTransport}
		}
	}
	if c != nil && slices.Contains(c.policy.Deny, capability) {
		return &DeniedError{Capability: capability}
	}
	return nil
}

// NewWithClock supplies the interval clock before the controls are published.
func NewWithClock(policy Policy, store BudgetStore, now func() time.Time) (*Controls, error) {
	c, err := New(policy, store)
	if err != nil {
		return nil, err
	}
	if now != nil {
		c.now = now
	}
	return c, nil
}

func (c *Controls) Allowed(capability Capability) bool { return c.Check(capability) == nil }

func (c *Controls) Active() bool {
	return c != nil && (len(c.policy.Deny) > 0 || c.policy.Limits != (Limits{}) || c.policy.Mail != nil || c.policy.AI != nil || c.policy.Imports != nil)
}
func (c *Controls) Limits() Limits {
	if c == nil {
		return Limits{}
	}
	return c.policy.Limits
}

type Snapshot struct {
	Deny                    []Capability `json:"deny"`
	Limits                  Limits       `json:"limits"`
	MailMode                string       `json:"mail_mode,omitempty"`
	MailSuspended           bool         `json:"mail_suspended,omitempty"`
	AIModels                []string     `json:"ai_models,omitempty"`
	AIDeniedFeatures        []string     `json:"ai_denied_features,omitempty"`
	AIMaxOutputTokens       int          `json:"ai_max_output_tokens,omitempty"`
	ImportAdmissionRequired bool         `json:"import_admission_required"`
}

func (c *Controls) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{Deny: []Capability{}}
	}
	view := Snapshot{Deny: slices.Clone(c.policy.Deny), Limits: c.policy.Limits, ImportAdmissionRequired: c.policy.Imports != nil}
	if view.Deny == nil {
		view.Deny = []Capability{}
	}
	for _, capability := range capabilities {
		if !c.Allowed(capability) && !slices.Contains(view.Deny, capability) {
			view.Deny = append(view.Deny, capability)
		}
	}
	if c.policy.Mail != nil {
		view.MailMode = c.policy.Mail.Mode
		view.MailSuspended = c.policy.Mail.Suspended
	}
	if c.policy.AI != nil {
		view.AIModels = slices.Clone(c.policy.AI.AllowedModels)
		view.AIDeniedFeatures = slices.Clone(c.policy.AI.DeniedFeatures)
		view.AIMaxOutputTokens = c.policy.AI.MaxOutputTokens
	}
	return view
}

func (c *Controls) Mail() *MailPolicy {
	if c == nil || c.policy.Mail == nil {
		return nil
	}
	policy := *c.policy.Mail
	return &policy
}

func (c *Controls) AI() *AIPolicy {
	if c == nil || c.policy.AI == nil {
		return nil
	}
	ai := *c.policy.AI
	ai.AllowedModels = slices.Clone(ai.AllowedModels)
	ai.DeniedFeatures = slices.Clone(ai.DeniedFeatures)
	return &ai
}

// Acquire never queues goroutines. A permit belongs to one operation, not a user.
func (c *Controls) Acquire(ctx context.Context, resource string, ceiling int) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || ceiling == 0 {
		return func() {}, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[resource] >= ceiling {
		return nil, &LimitError{Resource: resource}
	}
	c.active[resource]++
	var once sync.Once
	return func() { once.Do(func() { c.mu.Lock(); defer c.mu.Unlock(); c.active[resource]-- }) }, nil
}

type LimitError struct{ Resource string }

func (e *LimitError) Error() string { return "operator limit reached: " + e.Resource }

func (e *LimitError) Status() int {
	if e.Resource == "attachment_bytes" || e.Resource == "blob_bytes" {
		return 413
	}
	return 429
}
