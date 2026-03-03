package rules

import "fmt"

// Registry holds all registered rules. It is NOT safe for concurrent use;
// all registration must happen during initialization before any concurrent access.
type Registry struct {
	rules map[string]Rule
	order []string

	schemaRules map[string]SchemaRule
	schemaOrder []string
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		rules:       make(map[string]Rule),
		schemaRules: make(map[string]SchemaRule),
	}
}

// Register adds a rule to the registry. It returns an error if a rule
// with the same ID is already registered.
func (r *Registry) Register(rule Rule) error {
	id := rule.ID()
	if _, exists := r.rules[id]; exists {
		return fmt.Errorf("rule %s is already registered", id)
	}
	r.rules[id] = rule
	r.order = append(r.order, id)
	return nil
}

// Get returns a rule by its ID, or nil if not found.
func (r *Registry) Get(id string) Rule {
	return r.rules[id]
}

// All returns all registered rules in registration order.
func (r *Registry) All() []Rule {
	result := make([]Rule, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.rules[id])
	}
	return result
}

// RegisterSchema adds a schema rule to the registry. It returns an error if
// a schema rule with the same ID is already registered.
func (r *Registry) RegisterSchema(rule SchemaRule) error {
	id := rule.ID()
	if _, exists := r.schemaRules[id]; exists {
		return fmt.Errorf("schema rule %s is already registered", id)
	}
	r.schemaRules[id] = rule
	r.schemaOrder = append(r.schemaOrder, id)
	return nil
}

// AllSchema returns all registered schema rules in registration order.
func (r *Registry) AllSchema() []SchemaRule {
	result := make([]SchemaRule, 0, len(r.schemaOrder))
	for _, id := range r.schemaOrder {
		result = append(result, r.schemaRules[id])
	}
	return result
}

// DefaultRegistry returns a new registry with all built-in rules registered.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	mustRegister(reg, &SelectStarRule{})
	mustRegister(reg, &MissingWhereUpdateRule{})
	mustRegister(reg, &MissingWhereDeleteRule{})
	mustRegister(reg, &UnboundedSelectRule{})
	mustRegister(reg, &LikeLeadingWildcardRule{})
	mustRegister(reg, &SelectForUpdateNoWhereRule{})
	mustRegister(reg, &DestructiveDDLRule{})
	mustRegister(reg, &NonConcurrentIndexRule{})

	mustRegisterSchema(reg, &DroppedColumnRule{})
	mustRegisterSchema(reg, &MissingNotNullRule{})
	mustRegisterSchema(reg, &TypeMismatchRule{})
	mustRegisterSchema(reg, &TableNotFoundRule{})

	return reg
}

// mustRegister registers a rule and panics on duplicate IDs, which indicates
// a programming error in built-in rule wiring.
func mustRegister(reg *Registry, rule Rule) {
	if err := reg.Register(rule); err != nil {
		panic(err)
	}
}

// mustRegisterSchema registers a schema rule and panics on duplicate IDs.
func mustRegisterSchema(reg *Registry, rule SchemaRule) {
	if err := reg.RegisterSchema(rule); err != nil {
		panic(err)
	}
}
