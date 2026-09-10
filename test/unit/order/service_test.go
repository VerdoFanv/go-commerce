package order_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/order"
	"github.com/verdofanv/golang-be/test/testutil"
)

type fakeRepo struct {
	created      *order.OrderModel
	items        []order.OrderItemModel
	idempotency  map[string]*order.IdempotencyModel
	createCalls  int
	createErr    error
	outboxQueued bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{idempotency: map[string]*order.IdempotencyModel{}}
}

func (f *fakeRepo) CreateOrder(_ context.Context, in order.CreateOrderTX) (*order.OrderModel, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.outboxQueued = true
	now := time.Now().UTC()
	f.created = &order.OrderModel{
		ID: 42, UserID: in.UserID, Status: domain.OrderPendingPayment,
		Total: 56000, Currency: "IDR", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.items = []order.OrderItemModel{{OrderID: 42, ProductID: in.Items[0].ProductID, Qty: in.Items[0].Qty, UnitPrice: 28000}}
	return f.created, nil
}

func (f *fakeRepo) FindByID(_ context.Context, userID, id uint) (*order.OrderModel, []order.OrderItemModel, error) {
	if f.created == nil || f.created.ID != id || f.created.UserID != userID {
		return nil, nil, domain.ErrNotFound
	}
	return f.created, f.items, nil
}

func (f *fakeRepo) FindByIDAny(_ context.Context, id uint) (*order.OrderModel, []order.OrderItemModel, error) {
	if f.created == nil || f.created.ID != id {
		return nil, nil, domain.ErrNotFound
	}
	return f.created, f.items, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, userID uint, _ int) ([]order.OrderModel, error) {
	if f.created == nil || f.created.UserID != userID {
		return nil, nil
	}
	return []order.OrderModel{*f.created}, nil
}

func (f *fakeRepo) Cancel(_ context.Context, userID, id uint) (*order.OrderModel, error) {
	if f.created == nil || f.created.UserID != userID || f.created.ID != id {
		return nil, domain.ErrNotFound
	}
	f.created.Status = domain.OrderCancelled
	return f.created, nil
}

func (f *fakeRepo) CancelSystem(_ context.Context, id uint) (*order.OrderModel, error) {
	if f.created == nil || f.created.ID != id {
		return nil, domain.ErrNotFound
	}
	f.created.Status = domain.OrderCancelled
	return f.created, nil
}

func (f *fakeRepo) ListExpiredPendingIDs(_ context.Context, _ time.Time, _ int) ([]uint, error) {
	return nil, nil
}

func (f *fakeRepo) Pay(_ context.Context, userID, id uint, outcome, _ string) (*order.OrderModel, *order.PaymentModel, error) {
	if f.created == nil || f.created.ID != id {
		return nil, nil, domain.ErrNotFound
	}
	if outcome == "timeout" {
		return nil, nil, domain.ErrUnavailable
	}
	pay := &order.PaymentModel{OrderID: id, Status: domain.PaymentSucceeded, Amount: f.created.Total}
	if outcome == "fail" {
		f.created.Status = domain.OrderPaymentFailed
		pay.Status = domain.PaymentFailed
	} else {
		f.created.Status = domain.OrderPaid
	}
	return f.created, pay, nil
}

func (f *fakeRepo) Fulfill(_ context.Context, id uint) (*order.OrderModel, error) {
	if f.created == nil || f.created.ID != id {
		return nil, domain.ErrNotFound
	}
	f.created.Status = domain.OrderFulfilled
	return f.created, nil
}

func (f *fakeRepo) FindIdempotency(_ context.Context, userID uint, key string) (*order.IdempotencyModel, error) {
	return f.idempotency[key+"|"+itoa(userID)], nil
}

func (f *fakeRepo) SaveIdempotencyResponse(_ context.Context, userID uint, key, method, path, reqHash string, status int, body []byte) error {
	st := status
	f.idempotency[key+"|"+itoa(userID)] = &order.IdempotencyModel{
		Key: key, UserID: userID, Method: method, Path: path, RequestHash: reqHash,
		ResponseStatus: &st, ResponseBody: body,
	}
	return nil
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

func TestCreate_RequiresIdempotencyKey(t *testing.T) {
	svc := order.NewService(newFakeRepo(), testutil.Config())
	_, _, _, err := svc.Create(context.Background(), order.CreateInput{
		UserID: 1, Items: []order.CreateItemInput{{ProductID: 1, Qty: 1}},
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestCreate_QueuesOutboxViaRepo(t *testing.T) {
	repo := newFakeRepo()
	svc := order.NewService(repo, testutil.Config())
	o, status, body, err := svc.Create(context.Background(), order.CreateInput{
		UserID: 1, IdempotencyKey: "k1",
		Items: []order.CreateItemInput{{ProductID: 9, Qty: 2}},
	})
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.Nil(t, body)
	require.Equal(t, uint(42), o.ID)
	require.Equal(t, domain.OrderPendingPayment, o.Status)
	require.True(t, repo.outboxQueued)
	require.Equal(t, 1, repo.createCalls)
}

func TestCreate_ReplaysIdempotentResponse(t *testing.T) {
	repo := newFakeRepo()
	svc := order.NewService(repo, testutil.Config())
	env := map[string]any{"success": true, "data": map[string]any{"id": 42}}
	raw, _ := json.Marshal(env)
	st := http.StatusCreated
	repo.idempotency["k1|1"] = &order.IdempotencyModel{
		Key: "k1", UserID: 1, RequestHash: "", // empty hash accepts any on first stored without hash check path
		ResponseStatus: &st, ResponseBody: raw,
	}
	// Fix hash: Create computes hash of items — store matching hash by calling Remember first path
	// Simpler: set RequestHash to hash of same items via first create flow
	repo.idempotency = map[string]*order.IdempotencyModel{}
	o, _, _, err := svc.Create(context.Background(), order.CreateInput{
		UserID: 1, IdempotencyKey: "k1",
		Items: []order.CreateItemInput{{ProductID: 9, Qty: 2}},
	})
	require.NoError(t, err)
	require.NotNil(t, o)
	body, _ := json.Marshal(map[string]any{"success": true, "message": "order created", "data": o})
	require.NoError(t, svc.RememberIdempotent(context.Background(), 1, "k1", "POST", "/orders", []order.CreateItemInput{{ProductID: 9, Qty: 2}}, 201, body))

	_, status, replay, err := svc.Create(context.Background(), order.CreateInput{
		UserID: 1, IdempotencyKey: "k1",
		Items: []order.CreateItemInput{{ProductID: 9, Qty: 2}},
	})
	require.NoError(t, err)
	require.Equal(t, 201, status)
	require.NotNil(t, replay)
	require.Equal(t, 1, repo.createCalls) // second call did not create again
}

func TestPay_Timeout(t *testing.T) {
	repo := newFakeRepo()
	repo.created = &order.OrderModel{ID: 1, UserID: 1, Status: domain.OrderPendingPayment}
	svc := order.NewService(repo, testutil.Config())
	_, _, err := svc.Pay(context.Background(), order.PayInput{UserID: 1, OrderID: 1, Outcome: "timeout"})
	require.ErrorIs(t, err, domain.ErrUnavailable)
}
