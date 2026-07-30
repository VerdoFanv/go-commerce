package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func newService() (*auth.Service, *mocks.AuthRepository) {
	repo := mocks.NewAuthRepository()
	return auth.NewService(repo, testutil.Config()), repo
}

func TestRegister_Success(t *testing.T) {
	svc, _ := newService()

	result, err := svc.Register(context.Background(), auth.RegisterInput{
		Name:     "  Andi  ",
		Email:    "  Andi@Example.com ",
		Password: "secret1",
	})
	require.NoError(t, err)
	require.Equal(t, "Andi", result.User.Name)
	require.Equal(t, "andi@example.com", result.User.Email)
	require.NotEmpty(t, result.Tokens.AccessToken)
	require.NotEmpty(t, result.Tokens.RefreshToken)
}

func TestRegister_InvalidInput(t *testing.T) {
	svc, _ := newService()

	cases := []auth.RegisterInput{
		{Name: "", Email: "a@b.com", Password: "secret1"},
		{Name: "Andi", Email: "", Password: "secret1"},
		{Name: "Andi", Email: "a@b.com", Password: "123"},
	}
	for _, in := range cases {
		_, err := svc.Register(context.Background(), in)
		require.ErrorIs(t, err, domain.ErrInvalid, "input=%+v", in)
	}
}

func TestRegister_EmailTaken(t *testing.T) {
	svc, repo := newService()
	repo.CreateErr = domain.ErrEmailTaken

	_, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.ErrorIs(t, err, domain.ErrEmailTaken)
}

func TestLogin_Success(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.NoError(t, err)

	result, err := svc.Login(context.Background(), auth.LoginInput{
		Email: "ANDI@example.com", Password: "secret1",
	})
	require.NoError(t, err)
	require.Equal(t, "andi@example.com", result.User.Email)
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.NoError(t, err)

	_, err = svc.Login(context.Background(), auth.LoginInput{
		Email: "andi@example.com", Password: "wrong",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestLogin_UnknownEmail(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Login(context.Background(), auth.LoginInput{
		Email: "missing@example.com", Password: "secret1",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestRefresh_Success(t *testing.T) {
	svc, _ := newService()
	registered, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.NoError(t, err)

	tokens, err := svc.Refresh(context.Background(), registered.Tokens.RefreshToken)
	require.NoError(t, err)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
}

func TestRefresh_AccessTokenRejected(t *testing.T) {
	svc, _ := newService()
	registered, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), registered.Tokens.AccessToken)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestRefresh_Expired(t *testing.T) {
	svc, _ := newService()
	cfg := testutil.Config()
	token := testutil.SignExpiredToken(t, cfg.JWTSecret, 1, "refresh")

	_, err := svc.Refresh(context.Background(), token)
	require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestMe_Success(t *testing.T) {
	svc, _ := newService()
	registered, err := svc.Register(context.Background(), auth.RegisterInput{
		Name: "Andi", Email: "andi@example.com", Password: "secret1",
	})
	require.NoError(t, err)

	user, err := svc.Me(context.Background(), registered.User.ID)
	require.NoError(t, err)
	require.Equal(t, "andi@example.com", user.Email)
}

func TestMe_NotFound(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Me(context.Background(), 99)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
