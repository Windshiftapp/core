package models

// EnumEntity interface methods for StatusCategory
func (c *StatusCategory) GetID() int   { return c.ID }
func (c *StatusCategory) GetName() string { return c.Name }

// EnumEntity interface methods for Status
func (s *Status) GetID() int   { return s.ID }
func (s *Status) GetName() string { return s.Name }

// EnumEntity interface methods for IterationType
func (i *IterationType) GetID() int   { return i.ID }
func (i *IterationType) GetName() string { return i.Name }

// EnumEntity interface methods for MilestoneCategory
func (m *MilestoneCategory) GetID() int   { return m.ID }
func (m *MilestoneCategory) GetName() string { return m.Name }

// EnumEntity interface methods for Priority
func (p *Priority) GetID() int   { return p.ID }
func (p *Priority) GetName() string { return p.Name }

// EnumEntity interface methods for HierarchyLevel
func (h *HierarchyLevel) GetID() int   { return h.ID }
func (h *HierarchyLevel) GetName() string { return h.Name }

// EnumEntity interface methods for ContactRole
func (c *ContactRole) GetID() int   { return c.ID }
func (c *ContactRole) GetName() string { return c.Name }

// EnumEntity interface methods for CustomerOrganisation
func (c *CustomerOrganisation) GetID() int   { return c.ID }
func (c *CustomerOrganisation) GetName() string { return c.Name }

// EnumEntity interface methods for TimeProjectCategory
func (t *TimeProjectCategory) GetID() int   { return t.ID }
func (t *TimeProjectCategory) GetName() string { return t.Name }

// EnumEntity interface methods for TimeProject
func (t *TimeProject) GetID() int   { return t.ID }
func (t *TimeProject) GetName() string { return t.Name }

// EnumEntity interface methods for LinkType
func (l *LinkType) GetID() int   { return l.ID }
func (l *LinkType) GetName() string { return l.Name }

// EnumEntity interface methods for RequestType
func (r *RequestType) GetID() int   { return r.ID }
func (r *RequestType) GetName() string { return r.Name }

// EnumEntity interface methods for ChannelCategory
func (c *ChannelCategory) GetID() int   { return c.ID }
func (c *ChannelCategory) GetName() string { return c.Name }

// EnumEntity interface methods for CollectionCategory
func (c *CollectionCategory) GetID() int   { return c.ID }
func (c *CollectionCategory) GetName() string { return c.Name }

// EnumEntity interface methods for ItemType
func (i *ItemType) GetID() int    { return i.ID }
func (i *ItemType) GetName() string { return i.Name }
