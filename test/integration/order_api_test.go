package integration_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/auth"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/internal/http/order"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

type orderRepoStub struct {
	nextID uint
	orders map[uint]*order.OrderModel
	items  map[uint][]order.OrderItemModel
	idem   map[string]*order.IdempotencyModel
}

func newOrderRepoStub() *orderRepoStub {
	return &orderRepoStub{
		nextID: 1,
		orders: map[uint]*order.OrderModel{},
		items:  map[uint][]order.OrderItemModel{},
		idem:   map[string]*order.IdempotencyModel{},
	}
}

func (r *orderRepoStub) CreateOrder(_ context.Context, in order.CreateOrderTX) (*order.OrderModel, error) {
	id := r.nextID
	r.nextID++
	now := time.Now().UTC()
	m := &order.OrderModel{
		ID: id, UserID: in.UserID, Status: domain.OrderPendingPayment,
		Total: 28000, Currency: "IDR", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	r.orders[id] = m
	r.items[id] = []order.OrderItemModel{{
		OrderID: id, ProductID: in.Items[0].ProductID, Qty: in.Items[0].Qty, UnitPrice: 28000,
	}}
	return m, nil
}

func (r *orderRepoStub) FindByID(_ context.Context, userID, id uint) (*order.OrderModel, []order.OrderItemModel, error) {
	m, ok := r.orders[id]
	if !ok || m.UserID != userID {
		return nil, nil, domain.ErrNotFound
	}
	return m, r.items[id], nil
}

func (r *orderRepoStub) FindByIDAny(_ context.Context, id uint) (*order.OrderModel, []order.OrderItemModel, error) {
	m, ok := r.orders[id]
	if !ok {
		return nil, nil, domain.ErrNotFound
	}
	return m, r.items[id], nil
}

func (r *orderRepoStub) ListByUser(_ context.Context, userID uint, _ int) ([]order.OrderModel, error) {
	var out []order.OrderModel
	for _, m := range r.orders {
		if m.UserID == userID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (r *orderRepoStub) Cancel(_ context.Context, userID, id uint) (*order.OrderModel, error) {
	m, ok := r.orders[id]
	if !ok || m.UserID != userID {
		return nil, domain.ErrNotFound
	}
	next, err := domain.Transition(m.Status, domain.OrderCancelled)
	if err != nil {
		return nil, err
	}
	m.Status = next
	return m, nil
}

func (r *orderRepoStub) CancelSystem(_ context.Context, id uint) (*order.OrderModel, error) {
	m, ok := r.orders[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	next, err := domain.Transition(m.Status, domain.OrderCancelled)
	if err != nil {
		return nil, err
	}
	m.Status = next
	return m, nil
}

func (r *orderRepoStub) ListExpiredPendingIDs(_ context.Context, _ time.Time, _ int) ([]uint, error) {
	return nil, nil
}

func (r *orderRepoStub) Pay(_ context.Context, userID, id uint, outcome, _ string) (*order.OrderModel, *order.PaymentModel, error) {
	m, ok := r.orders[id]
	if !ok || m.UserID != userID {
		return nil, nil, domain.ErrNotFound
	}
	if outcome == "timeout" {
		return nil, nil, domain.ErrUnavailable
	}
	pay := &order.PaymentModel{OrderID: id, Amount: m.Total, Status: domain.PaymentSucceeded}
	if outcome == "fail" {
		next, err := domain.Transition(m.Status, domain.OrderPaymentFailed)
		if err != nil {
			return nil, nil, err
		}
		m.Status = next
		pay.Status = domain.PaymentFailed
		return m, pay, nil
	}
	next, err := domain.Transition(m.Status, domain.OrderPaid)
	if err != nil {
		return nil, nil, err
	}
	m.Status = next
	return m, pay, nil
}

func (r *orderRepoStub) Fulfill(_ context.Context, id uint) (*order.OrderModel, error) {
	m, ok := r.orders[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	next, err := domain.Transition(m.Status, domain.OrderFulfilled)
	if err != nil {
		return nil, err
	}
	m.Status = next
	return m, nil
}

func (r *orderRepoStub) FindIdempotency(_ context.Context, userID uint, key string) (*order.IdempotencyModel, error) {
	return r.idem[key+"/"+strconv.FormatUint(uint64(userID), 10)], nil
}

func (r *orderRepoStub) SaveIdempotencyResponse(_ context.Context, userID uint, key, method, path, reqHash string, status int, body []byte) error {
	st := status
	r.idem[key+"/"+strconv.FormatUint(uint64(userID), 10)] = &order.IdempotencyModel{
		Key: key, UserID: userID, Method: method, Path: path, RequestHash: reqHash,
		ResponseStatus: &st, ResponseBody: body,
	}
	return nil
}

func setupOrderAPI() (*gin.Engine, string) {
	cfg := testutil.Config()
	authRepo := mocks.NewAuthRepository()
	authHandler := auth.NewHandler(auth.NewService(authRepo, cfg, nil))
	orderHandler := order.NewHandler(order.NewService(newOrderRepoStub(), testutil.Config(), nil))

	app := testutil.NewRouter()
	api := app.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	authHandler.RegisterRoutes(api, cfg.JWTSecret)
	orderHandler.RegisterRoutes(api, cfg.JWTSecret)
	return app, cfg.APIKey
}

func TestOrderAPI_CreateGetCancel(t *testing.T) {
	app, apiKey := setupOrderAPI()
	access := registerAndLogin(t, app, apiKey)

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/orders", map[string]any{
		"items": []map[string]any{{"productId": 1, "qty": 2}},
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access),
		testutil.WithHeader("Idempotency-Key", "ord-1"))
	require.Equal(t, 201, status)
	created := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, domain.OrderPendingPayment, created["status"])
	id := int(created["id"].(float64))

	// Replay same key
	status, body2 := testutil.DoJSON(t, app, "POST", "/api/v1/orders", map[string]any{
		"items": []map[string]any{{"productId": 1, "qty": 2}},
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access),
		testutil.WithHeader("Idempotency-Key", "ord-1"))
	require.Equal(t, 201, status)
	replayed := testutil.DecodeData[map[string]any](t, body2)
	require.Equal(t, float64(id), replayed["id"])

	status, body = testutil.DoJSON(t, app, "GET", "/api/v1/orders/"+strconv.Itoa(id), nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)
	got := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, domain.OrderPendingPayment, got["status"])

	status, body = testutil.DoJSON(t, app, "POST", "/api/v1/orders/"+strconv.Itoa(id)+"/cancel", nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)
	cancelled := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, domain.OrderCancelled, cancelled["status"])
}
