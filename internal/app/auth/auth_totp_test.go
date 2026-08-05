package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/DanilenkA/awg-portal/internal/domain"
)

// fakeTotpUserManager is an in-memory TotpUserManager for tests.
type fakeTotpUserManager struct {
	users map[domain.UserIdentifier]*domain.User
}

func newFakeTotpUserManager() *fakeTotpUserManager {
	return &fakeTotpUserManager{users: make(map[domain.UserIdentifier]*domain.User)}
}

func (f *fakeTotpUserManager) GetUser(_ context.Context, id domain.UserIdentifier) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (f *fakeTotpUserManager) UpdateUserInternal(_ context.Context, user *domain.User) (*domain.User, error) {
	f.users[user.Identifier] = user
	return user, nil
}

func dbUser(id, email string) *domain.User {
	return &domain.User{
		Identifier: domain.UserIdentifier(id),
		Email:      email,
		Authentications: []domain.UserAuthentication{
			{UserIdentifier: domain.UserIdentifier(id), Source: domain.UserSourceDatabase},
		},
	}
}

func TestStartEnrollment_GeneratesSecretAndQR(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u1", "alice@example.com")

	enr, err := a.StartEnrollment(user)
	require.NoError(t, err)
	require.NotEmpty(t, enr.Secret)
	require.Contains(t, enr.URL, "otpauth://totp/")
	require.Contains(t, enr.URL, "alice@example.com")
	require.Greater(t, len(enr.QRCodeImage), 100, "QR PNG should be non-trivial")
	// PNG magic bytes
	require.Equal(t, byte(0x89), enr.QRCodeImage[0])
}

// Regression: users without an email must still enroll (AccountName falls back to Identifier).
func TestStartEnrollment_NoEmail_FallsBackToIdentifier(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("admin2@example.com", "") // empty email

	enr, err := a.StartEnrollment(user)
	require.NoError(t, err)
	require.NotEmpty(t, enr.Secret)
	require.Contains(t, enr.URL, "admin2@example.com", "AccountName should fall back to Identifier")
}

func TestStartEnrollment_RejectsOauthUser(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := &domain.User{
		Identifier: "u2",
		Email:      "bob@example.com",
		Authentications: []domain.UserAuthentication{
			{UserIdentifier: "u2", Source: domain.UserSourceOauth},
		},
	}

	_, err := a.StartEnrollment(user)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database users")
}

func TestConfirmEnrollment_ValidCode_EnablesAndReturnsRecoveryCodes(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u3", "carol@example.com")
	_, _ = m.UpdateUserInternal(context.Background(), user)

	enr, err := a.StartEnrollment(user)
	require.NoError(t, err)

	code, err := totp.GenerateCode(enr.Secret, time.Now())
	require.NoError(t, err)

	codes, err := a.ConfirmEnrollment(context.Background(), user, enr.Secret, code)
	require.NoError(t, err)
	require.Len(t, codes, recoveryCodeCount)

	// user persisted with 2FA enabled
	saved, err := m.GetUser(context.Background(), user.Identifier)
	require.NoError(t, err)
	require.True(t, saved.TotpEnabled)
	require.NotEmpty(t, saved.TotpSecret)

	// recovery codes stored as bcrypt hashes (JSON), not plaintext
	var hashes []string
	require.NoError(t, json.Unmarshal([]byte(saved.TotpRecoveryCodes), &hashes))
	require.Len(t, hashes, recoveryCodeCount)
	for _, h := range hashes {
		require.True(t, strings.HasPrefix(h, "$2"), "recovery code must be bcrypt-hashed")
	}
	// plaintext codes must not appear in storage
	for _, c := range codes {
		require.NotContains(t, string(saved.TotpRecoveryCodes), c)
	}
}

func TestConfirmEnrollment_InvalidCode_Fails(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u4", "dave@example.com")

	enr, err := a.StartEnrollment(user)
	require.NoError(t, err)

	_, err = a.ConfirmEnrollment(context.Background(), user, enr.Secret, "000000")
	require.Error(t, err)
	require.False(t, user.TotpEnabled)
}

func TestValidateCode_ValidTotp_Succeeds(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u5", "erin@example.com")
	_, _ = m.UpdateUserInternal(context.Background(), user)

	enr, _ := a.StartEnrollment(user)
	code, _ := totp.GenerateCode(enr.Secret, time.Now())
	_, err := a.ConfirmEnrollment(context.Background(), user, enr.Secret, code)
	require.NoError(t, err)

	// validate with a fresh code
	loginCode, err := totp.GenerateCode(string(user.TotpSecret), time.Now())
	require.NoError(t, err)
	require.NoError(t, a.ValidateCode(context.Background(), user, loginCode))
}

func TestValidateCode_ReplayRejected(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u6", "frank@example.com")
	_, _ = m.UpdateUserInternal(context.Background(), user)

	enr, _ := a.StartEnrollment(user)
	code, _ := totp.GenerateCode(enr.Secret, time.Now())
	_, err := a.ConfirmEnrollment(context.Background(), user, enr.Secret, code)
	require.NoError(t, err)

	// Use a code for the current step, then replay it.
	loginCode, _ := totp.GenerateCode(string(user.TotpSecret), time.Now())
	require.NoError(t, a.ValidateCode(context.Background(), user, loginCode))

	// Replaying the exact same code (same time-step) must be rejected.
	err = a.ValidateCode(context.Background(), user, loginCode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already used")
}

func TestValidateCode_NotEnabled_Fails(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u7", "grace@example.com")

	err := a.ValidateCode(context.Background(), user, "123456")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not enabled")
}

func TestRecoveryCode_SingleUse(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u8", "heidi@example.com")
	_, _ = m.UpdateUserInternal(context.Background(), user)

	enr, _ := a.StartEnrollment(user)
	code, _ := totp.GenerateCode(enr.Secret, time.Now())
	codes, err := a.ConfirmEnrollment(context.Background(), user, enr.Secret, code)
	require.NoError(t, err)
	require.NotEmpty(t, codes)

	// first use succeeds
	require.NoError(t, a.ValidateCode(context.Background(), user, codes[0]))

	// second use of the same recovery code fails (consumed)
	err = a.ValidateCode(context.Background(), user, codes[0])
	require.Error(t, err)
}

func TestDisable_WipesTotpState(t *testing.T) {
	m := newFakeTotpUserManager()
	a := NewTotpAuthenticator(m)
	user := dbUser("u9", "ivan@example.com")
	_, _ = m.UpdateUserInternal(context.Background(), user)

	enr, _ := a.StartEnrollment(user)
	code, _ := totp.GenerateCode(enr.Secret, time.Now())
	_, err := a.ConfirmEnrollment(context.Background(), user, enr.Secret, code)
	require.NoError(t, err)
	require.True(t, user.TotpEnabled)

	require.NoError(t, a.Disable(context.Background(), user))
	require.False(t, user.TotpEnabled)
	require.Empty(t, user.TotpSecret)
	require.Empty(t, user.TotpRecoveryCodes)
	require.Zero(t, user.TotpLastUsedStep)
}

func TestRecoveryCodeFormat(t *testing.T) {
	require.True(t, isRecoveryCodeFormat("ABCD-EFGH"))
	require.False(t, isRecoveryCodeFormat("123456"))
	require.False(t, isRecoveryCodeFormat("ABCDEFGH"))
}
