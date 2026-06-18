package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mrz1836/go-foundation/models"
)

func TestNewHookRunner(t *testing.T) {
	t.Parallel()

	runner := models.NewHookRunner()
	assert.NotNil(t, runner)
}

func TestHookRunner_WithOptions(t *testing.T) {
	t.Parallel()

	called := false
	hook := func(_ *gorm.DB, _ any) error {
		called = true
		return nil
	}

	runner := models.NewHookRunner(
		models.WithAfterCreate(hook),
	)

	err := runner.RunAfterCreate(nil, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestHookRunner_RunAfterCreate(t *testing.T) {
	t.Parallel()

	order := []string{}

	runner := models.NewHookRunner(
		models.WithAfterCreate(func(_ *gorm.DB, _ any) error {
			order = append(order, "first")
			return nil
		}),
		models.WithAfterCreate(func(_ *gorm.DB, _ any) error {
			order = append(order, "second")
			return nil
		}),
	)

	err := runner.RunAfterCreate(nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order)
}

func TestHookRunner_RunAfterUpdate(t *testing.T) {
	t.Parallel()

	called := false
	runner := models.NewHookRunner(
		models.WithAfterUpdate(func(_ *gorm.DB, _ any) error {
			called = true
			return nil
		}),
	)

	err := runner.RunAfterUpdate(nil, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestHookRunner_RunAfterDelete(t *testing.T) {
	t.Parallel()

	called := false
	runner := models.NewHookRunner(
		models.WithAfterDelete(func(_ *gorm.DB, _ any) error {
			called = true
			return nil
		}),
	)

	err := runner.RunAfterDelete(nil, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestHookRunner_StopsOnError(t *testing.T) {
	t.Parallel()

	secondCalled := false

	runner := models.NewHookRunner(
		models.WithAfterCreate(func(_ *gorm.DB, _ any) error {
			return models.ErrHook
		}),
		models.WithAfterCreate(func(_ *gorm.DB, _ any) error {
			secondCalled = true
			return nil
		}),
	)

	err := runner.RunAfterCreate(nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, models.ErrHook)
	assert.False(t, secondCalled, "second hook should not be called after error")
}

func TestHookRunner_EmptyRunner(t *testing.T) {
	t.Parallel()

	runner := models.NewHookRunner()

	// Should not error with no hooks
	assert.NoError(t, runner.RunAfterCreate(nil, nil))
	assert.NoError(t, runner.RunAfterUpdate(nil, nil))
	assert.NoError(t, runner.RunAfterDelete(nil, nil))
}

func TestDefaultHooks(t *testing.T) {
	t.Parallel()

	runner := models.DefaultHooks()
	assert.NotNil(t, runner)

	// Default hooks should run without error
	assert.NoError(t, runner.RunAfterCreate(nil, nil))
	assert.NoError(t, runner.RunAfterUpdate(nil, nil))
	assert.NoError(t, runner.RunAfterDelete(nil, nil))
}

func TestHookRunner_ReceivesModelAndTx(t *testing.T) {
	t.Parallel()

	type testModel struct {
		Name string
	}

	var receivedModel any

	runner := models.NewHookRunner(
		models.WithAfterCreate(func(_ *gorm.DB, model any) error {
			receivedModel = model
			return nil
		}),
	)

	model := &testModel{Name: "test"}
	err := runner.RunAfterCreate(nil, model)
	require.NoError(t, err)

	assert.Equal(t, model, receivedModel)
}
