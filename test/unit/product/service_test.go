package product_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/elasticsearch"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func newService() (*product.Service, *mocks.ProductRepository) {
	repo := mocks.NewProductRepository()
	// cache, publisher, search nil → service must still work without infra
	svc := product.NewService(repo, nil, nil, nil, testutil.Config())
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

func TestCreate_PublishesEvent(t *testing.T) {
	repo := mocks.NewProductRepository()
	publisher := mocks.NewEventPublisher()
	published := publisher.WaitChannel()
	svc := product.NewService(repo, nil, publisher, nil, testutil.Config())

	p, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("expected product.created event to be published")
	}

	event, ok := publisher.Last()
	require.True(t, ok)
	require.Equal(t, domain.EventProductCreated, event.Type)
	require.Equal(t, p.ID, event.Payload["id"])
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

	result, err := svc.List(context.Background(), 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "A", result.Items[0].Name)
	require.False(t, result.HasMore)
}

func TestList_CursorPagination(t *testing.T) {
	svc, _ := newService()
	for range 5 {
		_, err := svc.Create(context.Background(), product.CreateInput{UserID: 1, Name: "P", Price: 1, Stock: 1})
		require.NoError(t, err)
	}

	// Page 1: newest 2 of 5.
	page1, err := svc.List(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	require.Len(t, page1.Items, 2)
	require.True(t, page1.HasMore)
	require.NotZero(t, page1.NextCursor)

	// Page 2: continues where page 1 stopped, no overlap.
	page2, err := svc.List(context.Background(), 1, page1.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, page2.Items, 2)
	require.True(t, page2.HasMore)
	require.Less(t, page2.Items[0].ID, page1.Items[0].ID)

	// Page 3: last item, no more pages.
	page3, err := svc.List(context.Background(), 1, page2.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, page3.Items, 1)
	require.False(t, page3.HasMore)

	// No duplicates across pages.
	seen := map[uint]bool{}
	for _, p := range append(append(page1.Items, page2.Items...), page3.Items...) {
		require.False(t, seen[p.ID], "duplicate product id %d across pages", p.ID)
		seen[p.ID] = true
	}
}

func TestSearch_DisabledWithoutElasticsearch(t *testing.T) {
	svc, _ := newService() // search engine nil
	_, err := svc.Search(context.Background(), "kopi", 20)
	require.ErrorIs(t, err, domain.ErrUnavailable)
}

func TestSearch_EmptyQuery(t *testing.T) {
	repo := mocks.NewProductRepository()
	svc := product.NewService(repo, nil, nil, &fakeSearchEngine{}, testutil.Config())
	_, err := svc.Search(context.Background(), "   ", 20)
	require.ErrorIs(t, err, domain.ErrInvalid)
}

// fakeSearchEngine satisfies product.SearchEngine without Elasticsearch.
type fakeSearchEngine struct{}

func (f *fakeSearchEngine) IndexProduct(context.Context, domain.Product) error { return nil }
func (f *fakeSearchEngine) DeleteProduct(context.Context, uint) error          { return nil }
func (f *fakeSearchEngine) Search(_ context.Context, _ string, _ int) ([]elasticsearch.ProductDocument, error) {
	return nil, nil
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

	require.NoError(t, svc.Delete(context.Background(), 1, domain.RoleUser, created.ID))
	_, err = svc.GetByID(context.Background(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_ForbiddenForOtherUser(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), 2, domain.RoleUser, created.ID)
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestDelete_AdminBypassesOwnership(t *testing.T) {
	svc, _ := newService()
	created, err := svc.Create(context.Background(), product.CreateInput{
		UserID: 1, Name: "Kopi", Price: 1000, Stock: 1,
	})
	require.NoError(t, err)

	// Admin (user 99) deletes user 1's product — moderation power.
	require.NoError(t, svc.Delete(context.Background(), 99, domain.RoleAdmin, created.ID))
	_, err = svc.GetByID(context.Background(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
