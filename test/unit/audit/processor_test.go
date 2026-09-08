package audit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/worker/audit"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
)

// fakeStore fails a configurable number of times before succeeding.
type fakeStore struct {
	failuresLeft int
	inserted     []audit.Record
}

func (f *fakeStore) Insert(_ context.Context, record audit.Record) error {
	if f.failuresLeft > 0 {
		f.failuresLeft--
		return errors.New("transient mongo error")
	}
	f.inserted = append(f.inserted, record)
	return nil
}

func TestHandle_PersistsOnFirstTry(t *testing.T) {
	store := &fakeStore{}
	processor := audit.NewProcessor(nil, nil, store)

	msg := kafka.Message{Event: kafka.NewEvent("product.created", map[string]any{"id": 1})}
	require.NoError(t, processor.Handle(context.Background(), msg))
	require.Len(t, store.inserted, 1)
	require.Equal(t, "product.created", store.inserted[0].Type)
}

func TestHandle_RetriesThenSucceeds(t *testing.T) {
	store := &fakeStore{failuresLeft: 2}
	processor := audit.NewProcessor(nil, nil, store)

	msg := kafka.Message{Event: kafka.NewEvent("product.created", map[string]any{"id": 1})}
	require.NoError(t, processor.Handle(context.Background(), msg))
	require.Len(t, store.inserted, 1)
}

func TestHandle_ExhaustsRetriesThenNeedsDLQ(t *testing.T) {
	store := &fakeStore{failuresLeft: 99}
	// No DLQ producer → Handle must surface the failure so the offset isn't committed.
	processor := audit.NewProcessor(nil, nil, store)

	msg := kafka.Message{Event: kafka.NewEvent("product.created", map[string]any{"id": 1})}
	err := processor.Handle(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no DLQ producer")
}

func TestHandle_PoisonPillGoesToDLQ(t *testing.T) {
	store := &fakeStore{}
	// Zero-value event (undecodable upstream) must skip retries entirely.
	processor := audit.NewProcessor(nil, nil, store)

	err := processor.Handle(context.Background(), kafka.Message{})
	require.Error(t, err) // fails only because DLQ producer is nil — but store untouched
	require.Empty(t, store.inserted)
}
