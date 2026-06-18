package models

import "gorm.io/gorm"

// HookFunc is the signature for lifecycle hooks.
type HookFunc func(tx *gorm.DB, model any) error

// HookRunner manages and executes lifecycle hooks.
// It allows models to run a chain of hooks and optionally add custom logic.
type HookRunner struct {
	afterCreate []HookFunc
	afterUpdate []HookFunc
	afterDelete []HookFunc
}

// HookOption configures a HookRunner.
type HookOption func(*HookRunner)

// NewHookRunner creates a HookRunner with the given options.
func NewHookRunner(opts ...HookOption) *HookRunner {
	r := &HookRunner{}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// WithAfterCreate adds an AfterCreate hook to the runner.
func WithAfterCreate(h HookFunc) HookOption {
	return func(r *HookRunner) {
		r.afterCreate = append(r.afterCreate, h)
	}
}

// WithAfterUpdate adds an AfterUpdate hook to the runner.
func WithAfterUpdate(h HookFunc) HookOption {
	return func(r *HookRunner) {
		r.afterUpdate = append(r.afterUpdate, h)
	}
}

// WithAfterDelete adds an AfterDelete hook to the runner.
func WithAfterDelete(h HookFunc) HookOption {
	return func(r *HookRunner) {
		r.afterDelete = append(r.afterDelete, h)
	}
}

// RunAfterCreate executes all AfterCreate hooks in order.
// Returns the first error encountered, or nil if all hooks succeed.
func (r *HookRunner) RunAfterCreate(tx *gorm.DB, model any) error {
	for _, h := range r.afterCreate {
		if err := h(tx, model); err != nil {
			return err
		}
	}

	return nil
}

// RunAfterUpdate executes all AfterUpdate hooks in order.
// Returns the first error encountered, or nil if all hooks succeed.
func (r *HookRunner) RunAfterUpdate(tx *gorm.DB, model any) error {
	for _, h := range r.afterUpdate {
		if err := h(tx, model); err != nil {
			return err
		}
	}

	return nil
}

// RunAfterDelete executes all AfterDelete hooks in order.
// Returns the first error encountered, or nil if all hooks succeed.
func (r *HookRunner) RunAfterDelete(tx *gorm.DB, model any) error {
	for _, h := range r.afterDelete {
		if err := h(tx, model); err != nil {
			return err
		}
	}

	return nil
}

// DefaultHooks returns an empty HookRunner with no pre-registered hooks.
// Models can use this as a starting point and extend with custom logic
// via WithAfterCreate, WithAfterUpdate, and WithAfterDelete.
func DefaultHooks() *HookRunner {
	return NewHookRunner()
}
