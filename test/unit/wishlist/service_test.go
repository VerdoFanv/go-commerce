package wishlist_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/internal/wishlist"
	"github.com/verdofanv/golang-be/test/testutil"
)

type memProducts struct {
	byID map[uint]*product.ProductModel
}

func (m *memProducts) FindByID(_ context.Context, id uint) (*product.ProductModel, error) {
	p, ok := m.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return p, nil
}

type memWishlists struct {
	rows   []wishlist.WishlistModel
	nextID uint
}

func (m *memWishlists) Create(_ context.Context, row *wishlist.WishlistModel) error {
	for _, r := range m.rows {
		if r.UserID == row.UserID && r.ProductID == row.ProductID {
			return domain.ErrConflict
		}
	}
	m.nextID++
	row.ID = m.nextID
	m.rows = append(m.rows, *row)
	return nil
}

func (m *memWishlists) ListByUser(_ context.Context, userID uint) ([]wishlist.WishlistModel, error) {
	var out []wishlist.WishlistModel
	for _, r := range m.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memWishlists) Delete(_ context.Context, userID, id uint) error {
	for i, r := range m.rows {
		if r.ID == id && r.UserID == userID {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *memWishlists) CountByUser(_ context.Context, userID uint) (int64, error) {
	var n int64
	for _, r := range m.rows {
		if r.UserID == userID {
			n++
		}
	}
	return n, nil
}

func TestWishlist_AddListCountRemove(t *testing.T) {
	cfg := testutil.Config()
	products := &memProducts{byID: map[uint]*product.ProductModel{
		7: {ID: 7, UserID: 1, Name: "Kopi", Price: 28000},
	}}
	repo := &memWishlists{}
	svc := wishlist.NewService(repo, products, nil, cfg)

	item, err := svc.Add(context.Background(), wishlist.AddInput{UserID: 2, ProductID: 7, Note: "coba"})
	require.NoError(t, err)
	require.Equal(t, uint(7), item.ProductID)
	require.Equal(t, "Kopi", item.ProductName)

	_, err = svc.Add(context.Background(), wishlist.AddInput{UserID: 2, ProductID: 7})
	require.ErrorIs(t, err, domain.ErrConflict)

	list, err := svc.List(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, list, 1)

	n, err := svc.Count(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	require.NoError(t, svc.Remove(context.Background(), 2, item.ID))
	n, err = svc.Count(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)
}
