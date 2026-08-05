package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/DanilenkA/awg-portal/internal/domain"
)

// TOTP two-factor authentication: a second factor on top of any first factor
// (password or passkey). Applies to database-source users only (opt-in) —
// OAuth/LDAP/OIDC users manage 2FA at their external provider.
//
// Security contract (see handoffs/plan-2fa-totp.md §6):
//   - secrets are stored encrypted (gorm serializer:encstr) and never logged
//   - validation uses Skew=1 (±30s) per RFC 6238; never widen without reason
//   - replay protection via TotpLastUsedStep (a code cannot be reused)
//   - rate limiting is enforced by the caller (endpoint layer), not here

const (
	totpIssuer        = "awg-portal"
	totpPeriod        = 30
	totpSecretSize    = 20 // 160-bit, Google Authenticator compatible
	recoveryCodeCount = 8  // number of single-use recovery codes issued at enrollment
)

// TotpUserManager is the subset of the user manager required by the TOTP authenticator.
type TotpUserManager interface {
	// GetUser returns a user by its identifier.
	GetUser(context.Context, domain.UserIdentifier) (*domain.User, error)
	// UpdateUserInternal updates an existing user in the database.
	UpdateUserInternal(ctx context.Context, user *domain.User) (*domain.User, error)
}

// TotpAuthenticator implements TOTP enrollment and validation.
type TotpAuthenticator struct {
	users TotpUserManager
}

// NewTotpAuthenticator creates a new TOTP authenticator.
func NewTotpAuthenticator(users TotpUserManager) *TotpAuthenticator {
	return &TotpAuthenticator{users: users}
}

// totpValidateOpts returns the validation options matching enrollment (Skew=1, RFC 6238).
func totpValidateOpts() totp.ValidateOpts {
	return totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
}

// StartEnrollment generates a fresh TOTP secret and QR code for the user.
// The secret is NOT persisted yet — it is returned for display and must be
// confirmed via ConfirmEnrollment before 2FA becomes active.
func (a *TotpAuthenticator) StartEnrollment(user *domain.User) (*domain.TotpEnrollment, error) {
	if !user.CanUseTotp() {
		return nil, errors.New("totp is only available for database users")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: user.Email,
		Period:      totpPeriod,
		SecretSize:  totpSecretSize,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate totp key: %w", err)
	}

	img, err := key.Image(200, 200)
	if err != nil {
		return nil, fmt.Errorf("failed to render totp qr code: %w", err)
	}

	png, err := encodePNG(img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode totp qr code: %w", err)
	}

	return &domain.TotpEnrollment{
		Secret:      key.Secret(),
		URL:         key.URL(),
		QRCodeImage: png,
	}, nil
}

// ConfirmEnrollment verifies the first TOTP code against the pending secret and,
// on success, enables 2FA: persists the (encrypted) secret and issues recovery codes.
// The pending secret is supplied by the caller (from the enrollment session), not stored server-side beforehand.
// Returns the plaintext recovery codes exactly once — they must be shown to the user and never stored unhashed.
func (a *TotpAuthenticator) ConfirmEnrollment(ctx context.Context, user *domain.User, pendingSecret, code string) ([]string, error) {
	if !user.CanUseTotp() {
		return nil, errors.New("totp is only available for database users")
	}
	if pendingSecret == "" {
		return nil, errors.New("missing pending totp secret")
	}

	if !totp.Validate(code, pendingSecret) {
		return nil, errors.New("invalid totp code")
	}

	// Generate single-use recovery codes; store only their bcrypt hashes.
	codes, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recovery codes: %w", err)
	}
	hashes, err := hashRecoveryCodes(codes)
	if err != nil {
		return nil, fmt.Errorf("failed to hash recovery codes: %w", err)
	}
	hashesJSON, err := json.Marshal(hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to encode recovery codes: %w", err)
	}

	user.TotpSecret = domain.PrivateString(pendingSecret)
	user.TotpRecoveryCodes = domain.PrivateString(string(hashesJSON))
	user.TotpLastUsedStep = 0
	user.TotpEnabled = true

	if _, err := a.users.UpdateUserInternal(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save totp enrollment: %w", err)
	}

	return codes, nil
}

// ValidateCode checks a TOTP code (or a recovery code) for a 2FA-enabled user.
// On TOTP success it records the used time-step for replay protection.
// On recovery-code success it consumes (removes) that code.
// The caller must enforce rate limiting before invoking this.
func (a *TotpAuthenticator) ValidateCode(ctx context.Context, user *domain.User, code string) error {
	if !user.TotpEnabled {
		return errors.New("totp is not enabled for this user")
	}
	if user.TotpSecret == "" {
		return errors.New("missing totp secret")
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("empty totp code")
	}

	// Try recovery code first (format xxxx-xxxx), then a 6-digit TOTP code.
	if isRecoveryCodeFormat(code) {
		return a.consumeRecoveryCode(ctx, user, code)
	}

	ok, step, err := validateTotpWithStep(code, string(user.TotpSecret))
	if err != nil {
		return fmt.Errorf("totp validation failed: %w", err)
	}
	if !ok {
		return errors.New("invalid totp code")
	}

	// Replay protection: reject a code from an already-used time-step.
	if step <= user.TotpLastUsedStep && user.TotpLastUsedStep != 0 {
		return errors.New("totp code already used")
	}

	user.TotpLastUsedStep = step
	// Record usage with admin context: the pending-2FA session has no user context,
	// so the permission check in UpdateUserInternal would otherwise reject the write.
	if _, err := a.users.UpdateUserInternal(domain.SetUserInfo(ctx, domain.SystemAdminContextUserInfo()), user); err != nil {
		return fmt.Errorf("failed to record totp usage: %w", err)
	}

	return nil
}

// Disable turns off 2FA and wipes the secret and recovery codes.
func (a *TotpAuthenticator) Disable(ctx context.Context, user *domain.User) error {
	user.TotpEnabled = false
	user.TotpSecret = ""
	user.TotpRecoveryCodes = ""
	user.TotpLastUsedStep = 0

	if _, err := a.users.UpdateUserInternal(ctx, user); err != nil {
		return fmt.Errorf("failed to disable totp: %w", err)
	}
	return nil
}

// validateTotpWithStep validates the code and returns the matching time-step (for replay protection).
func validateTotpWithStep(passcode, secret string) (bool, int64, error) {
	opts := totpValidateOpts()
	now := time.Now().UTC()
	counter := now.Unix() / int64(opts.Period)

	// Check current step and the RFC-allowed skew window (±1).
	candidates := []int64{counter, counter + 1, counter - 1}
	for _, c := range candidates {
		generated, err := totp.GenerateCodeCustom(secret, time.Unix(c*int64(opts.Period), 0).UTC(), opts)
		if err != nil {
			return false, 0, err
		}
		if subtleCompare(generated, passcode) {
			return true, c, nil
		}
	}
	return false, 0, nil
}

// consumeRecoveryCode validates and removes a single-use recovery code.
func (a *TotpAuthenticator) consumeRecoveryCode(ctx context.Context, user *domain.User, code string) error {
	var hashes []string
	if err := json.Unmarshal([]byte(user.TotpRecoveryCodes), &hashes); err != nil {
		return fmt.Errorf("failed to read recovery codes: %w", err)
	}

	normalized := normalizeRecoveryCode(code)
	remaining := make([]string, 0, len(hashes))
	matched := false
	for _, h := range hashes {
		if !matched && bcrypt.CompareHashAndPassword([]byte(h), []byte(normalized)) == nil {
			matched = true // consume: drop this hash
			continue
		}
		remaining = append(remaining, h)
	}
	if !matched {
		return errors.New("invalid recovery code")
	}

	hashesJSON, err := json.Marshal(remaining)
	if err != nil {
		return fmt.Errorf("failed to encode recovery codes: %w", err)
	}
	user.TotpRecoveryCodes = domain.PrivateString(string(hashesJSON))
	if _, err := a.users.UpdateUserInternal(domain.SetUserInfo(ctx, domain.SystemAdminContextUserInfo()), user); err != nil {
		return fmt.Errorf("failed to update recovery codes: %w", err)
	}
	return nil
}

// generateRecoveryCodes creates n codes in the xxxx-xxxx format (Crockford base32, unambiguous alphabet).
func generateRecoveryCodes(n int) ([]string, error) {
	const alphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ" // Crockford base32, no 0/O/1/I/L
	codes := make([]string, 0, n)
	buf := make([]byte, 8)
	for i := 0; i < n; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		var b strings.Builder
		for j := 0; j < 8; j++ {
			if j == 4 {
				b.WriteByte('-')
			}
			b.WriteByte(alphabet[int(buf[j])%len(alphabet)])
		}
		codes = append(codes, b.String())
	}
	return codes, nil
}

// hashRecoveryCodes returns bcrypt hashes of the normalized codes.
func hashRecoveryCodes(codes []string) ([]string, error) {
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		h, err := bcrypt.GenerateFromPassword([]byte(normalizeRecoveryCode(c)), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, string(h))
	}
	return hashes, nil
}

// isRecoveryCodeFormat reports whether the input looks like a recovery code (xxxx-xxxx).
func isRecoveryCodeFormat(code string) bool {
	return len(code) == 9 && code[4] == '-'
}

// normalizeRecoveryCode upper-cases and trims a recovery code for comparison.
func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// subtleCompare compares two strings in constant time.
func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// encodePNG renders an image as PNG bytes (for the TOTP QR code).
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
