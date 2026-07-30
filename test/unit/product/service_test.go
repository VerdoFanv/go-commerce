package product_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func newService() (*product.Service, *mocks.ProductRepository) {
	repo := mocks.NewProductRepository()
	// cache & mq nil → service must still work without infra
	svc := product.NewService(repo, nil, nil, testutil.Config())
	return svc, repo
}

func TestCreate_Success(t *testing.T) {
	svc, _ := newService()

	p, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "  Kopi  ", Description: " iced ", Price: 28000, Stock: 10,
	})
	require.NoError(t, err)
	require.Equal(t, "Kopi", p.Name)
	require.Equal(t, "iced", p.Description)
	require.Equal(t, uint(1), p.UserID)
	require.NotZero(t, p.ID)
}

func TestCreate_InvalidInput(t *testing.T) {
	svc, _ := newService()

	cases := []product.CreateInput{
		{UserID: 1, Name: "", Price: 1, Stock: 0},
		{UserID: 1, Name: "X", Price: -1, Stock: 0},
		{UserID: 1, Name: "X", Price: 1, Stock: -1},
	}
	for _, in := range cases {
		_, err := svc.Create(context.Background(), in)
		require.ErrorIs(t, err, domain.ErrInvalid, "input=%+v", in)
	}
}

func TestGetByID_Success(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	got, err := svc.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "Kopi", got.Name)
}

func TestGetByID_NotFound(t *testing.T) {
	svc, _ := newService()
	_, err := svc.GetByID(context.Background(), 999)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestList_ByUser(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Create(context.Background(), product.CreateInput{UserID: 1, Name: "A", Price: 1, Stock: 1})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), product.CreateInput{UserID: 2, Name: "B", Price: 1, Stock: 1})
	require.NoError(t, err)

	list, err := svc.List(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "A", list[0].Name)
}

func TestUpdate_Success(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	name := "Kopi Susu"
	price := 28000.0
	updated, err := svc.Update(context.Background(), 1, created.ID, product.UpdateInput{
		Name:  &name,
		Price: &price,
	})
	require.NoError(t, err)
	require.Equal(t, "Kopi Susu", updated.Name)
	require.Equal(t, 28000.0, updated.Price)
}

func TestUpdate_Forbidden(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	name := "Hack"
	_, err = svc.Update(context.Background(), 2, created.ID, product.UpdateInput{Name: &name})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestUpdate_InvalidName(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	empty := "   "
	_, err = svc.Update(context.Background(), 1, created.ID, product.UpdateInput{Name: &empty})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestDelete_Success(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(context.Background(), 1, created.ID))
	_, err = svc.GetByID(context.Background(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_Forbidden(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), 2, created.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
