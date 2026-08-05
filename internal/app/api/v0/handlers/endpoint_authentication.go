package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/routegroup"

	"github.com/DanilenkA/awg-portal/internal/app/api/core/request"
	"github.com/DanilenkA/awg-portal/internal/app/api/core/respond"
	"github.com/DanilenkA/awg-portal/internal/app/api/v0/model"
	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
)

type AuthenticationService interface {
	// GetExternalLoginProviders returns a list of all available external login providers.
	GetExternalLoginProviders(_ context.Context) []domain.LoginProviderInfo
	// PlainLogin authenticates a user with a username and password.
	PlainLogin(ctx context.Context, username, password string) (*domain.User, error)
	// GetUser returns a user by its identifier.
	GetUser(ctx context.Context, id domain.UserIdentifier) (*domain.User, error)
	// OauthLoginStep1 initiates the OAuth login flow.
	OauthLoginStep1(_ context.Context, providerId string) (authCodeUrl, state, nonce string, err error)
	// OauthLoginStep2 completes the OAuth login flow and logins the user in.
	OauthLoginStep2(ctx context.Context, providerId, nonce, code string) (*domain.User, string, error)
	// OauthProviderLogoutUrl returns an IdP logout URL for the given provider if supported.
	OauthProviderLogoutUrl(providerId, idTokenHint, postLogoutRedirectUri string) (string, bool)
}

type WebAuthnService interface {
	Enabled() bool
	StartWebAuthnRegistration(ctx context.Context, userId domain.UserIdentifier) (
		responseOptions []byte,
		sessionData []byte,
		err error,
	)
	FinishWebAuthnRegistration(
		ctx context.Context,
		userId domain.UserIdentifier,
		name string,
		sessionDataAsJSON []byte,
		r *http.Request,
	) ([]domain.UserWebauthnCredential, error)
	GetCredentials(
		ctx context.Context,
		userId domain.UserIdentifier,
	) ([]domain.UserWebauthnCredential, error)
	RemoveCredential(
		ctx context.Context,
		userId domain.UserIdentifier,
		credentialIdBase64 string,
	) ([]domain.UserWebauthnCredential, error)
	UpdateCredential(
		ctx context.Context,
		userId domain.UserIdentifier,
		credentialIdBase64 string,
		name string,
	) ([]domain.UserWebauthnCredential, error)
	StartWebAuthnLogin(_ context.Context) (
		optionsAsJSON []byte,
		sessionDataAsJSON []byte,
		err error,
	)
	FinishWebAuthnLogin(
		ctx context.Context,
		sessionDataAsJSON []byte,
		r *http.Request,
	) (*domain.User, error)
}

// TotpService is the interface for TOTP 2FA (second factor on top of any first factor).
type TotpService interface {
	// StartEnrollment generates a fresh TOTP secret/QR for the user (not persisted yet).
	StartEnrollment(user *domain.User) (*domain.TotpEnrollment, error)
	// ConfirmEnrollment verifies the first code and enables 2FA, returning recovery codes.
	ConfirmEnrollment(ctx context.Context, user *domain.User, pendingSecret, code string) ([]string, error)
	// ValidateCode validates a TOTP or recovery code for a 2FA-enabled user (caller enforces rate limiting).
	ValidateCode(ctx context.Context, user *domain.User, code string) error
	// Disable turns off 2FA for the user.
	Disable(ctx context.Context, user *domain.User) error
}

type AuthEndpoint struct {
	cfg           *config.Config
	authService   AuthenticationService
	authenticator Authenticator
	session       Session
	validate      Validator
	webAuthn      WebAuthnService
	totp          TotpService
}

func NewAuthEndpoint(
	cfg *config.Config,
	authenticator Authenticator,
	session Session,
	validator Validator,
	authService AuthenticationService,
	webAuthn WebAuthnService,
	totp TotpService,
) AuthEndpoint {
	return AuthEndpoint{
		cfg:           cfg,
		authService:   authService,
		authenticator: authenticator,
		session:       session,
		validate:      validator,
		webAuthn:      webAuthn,
		totp:          totp,
	}
}

func (e AuthEndpoint) GetName() string {
	return "AuthEndpoint"
}

func (e AuthEndpoint) RegisterRoutes(g *routegroup.Bundle) {
	apiGroup := g.Mount("/auth")

	apiGroup.HandleFunc("GET /providers", e.handleExternalLoginProvidersGet())
	apiGroup.HandleFunc("GET /session", e.handleSessionInfoGet())

	apiGroup.HandleFunc("GET /login/{provider}/init", e.handleOauthInitiateGet())
	apiGroup.HandleFunc("GET /login/{provider}/callback", e.handleOauthCallbackGet())

	apiGroup.HandleFunc("POST /webauthn/login/start", e.handleWebAuthnLoginStart())
	apiGroup.HandleFunc("POST /webauthn/login/finish", e.handleWebAuthnLoginFinish())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("GET /webauthn/credentials",
		e.handleWebAuthnCredentialsGet())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /webauthn/register/start",
		e.handleWebAuthnRegisterStart())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /webauthn/register/finish",
		e.handleWebAuthnRegisterFinish())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("DELETE /webauthn/credential/{id}",
		e.handleWebAuthnCredentialsDelete())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("PUT /webauthn/credential/{id}",
		e.handleWebAuthnCredentialsPut())

	apiGroup.HandleFunc("POST /login", e.handleLoginPost())
	apiGroup.HandleFunc("POST /login/totp", e.handleLoginTotpPost())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /logout", e.handleLogoutPost())

	// TOTP 2FA management (opt-in, database users only)
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("GET /totp/status", e.handleTotpStatusGet())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /totp/enroll/start", e.handleTotpEnrollStart())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /totp/enroll/confirm", e.handleTotpEnrollConfirm())
	apiGroup.With(e.authenticator.LoggedIn()).HandleFunc("POST /totp/disable", e.handleTotpDisablePost())
}

// handleExternalLoginProvidersGet returns a gorm Handler function.
//
// @ID auth_handleExternalLoginProvidersGet
// @Tags Authentication
// @Summary Get all available external login providers.
// @Produce json
// @Success 200 {object} []model.LoginProviderInfo
// @Router /auth/providers [get]
func (e AuthEndpoint) handleExternalLoginProvidersGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providers := e.authService.GetExternalLoginProviders(r.Context())

		respond.JSON(w, http.StatusOK, model.NewLoginProviderInfos(providers))
	}
}

// handleSessionInfoGet returns a gorm Handler function.
//
// @ID auth_handleSessionInfoGet
// @Tags Authentication
// @Summary Get information about the currently logged-in user.
// @Produce json
// @Success 200 {object} []model.SessionInfo
// @Failure 500 {object} model.Error
// @Router /auth/session [get]
func (e AuthEndpoint) handleSessionInfoGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())

		var loggedInUid *string
		var firstname *string
		var lastname *string
		var email *string

		if currentSession.LoggedIn {
			uid := currentSession.UserIdentifier
			f := currentSession.Firstname
			l := currentSession.Lastname
			e := currentSession.Email
			loggedInUid = &uid
			firstname = &f
			lastname = &l
			email = &e
		}

		respond.JSON(w, http.StatusOK, model.SessionInfo{
			LoggedIn:       currentSession.LoggedIn,
			IsAdmin:        currentSession.IsAdmin,
			UserIdentifier: loggedInUid,
			UserFirstname:  firstname,
			UserLastname:   lastname,
			UserEmail:      email,
		})
	}
}

// handleOauthInitiateGet returns a gorm Handler function.
//
// @ID auth_handleOauthInitiateGet
// @Tags Authentication
// @Summary Initiate the OAuth login flow.
// @Produce json
// @Success 200 {object} []model.LoginProviderInfo
// @Router /auth/login/{provider}/init [get]
func (e AuthEndpoint) handleOauthInitiateGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())

		autoRedirect, _ := strconv.ParseBool(request.QueryDefault(r, "redirect", "false"))
		returnTo := request.Query(r, "return")
		provider := request.Path(r, "provider")

		var returnUrl *url.URL
		redirectToReturn := func(loginState string) {
			respond.Redirect(w, r, http.StatusFound, e.returnUrlWithLoginState(returnUrl, loginState))
		}

		if returnTo != "" {
			if !e.isValidReturnUrl(returnTo) {
				respond.JSON(w, http.StatusBadRequest,
					model.Error{Code: http.StatusBadRequest, Message: "invalid return URL"})
				return
			}
			u, err := url.Parse(returnTo)
			if err != nil {
				respond.JSON(w, http.StatusBadRequest,
					model.Error{Code: http.StatusBadRequest, Message: "invalid return URL"})
				return
			}
			returnUrl = u
		}

		if currentSession.LoggedIn {
			if autoRedirect && returnUrl != nil {
				redirectToReturn("success")
			} else {
				respond.JSON(w, http.StatusBadRequest,
					model.Error{Code: http.StatusBadRequest, Message: "already logged in"})
			}
			return
		}

		authCodeUrl, state, nonce, err := e.authService.OauthLoginStep1(context.Background(), provider)
		if err != nil {
			slog.Debug("failed to create oauth auth code URL",
				"provider", provider, "error", err)
			if autoRedirect && returnUrl != nil {
				redirectToReturn("err")
			} else {
				respond.JSON(w, http.StatusInternalServerError,
					model.Error{Code: http.StatusInternalServerError, Message: err.Error()})
			}
			return
		}

		authSession := e.session.GetData(r.Context())
		authSession.OauthState = state
		authSession.OauthNonce = nonce
		authSession.OauthProvider = provider
		authSession.OauthReturnTo = returnTo
		e.session.SetData(r.Context(), authSession)

		if autoRedirect {
			respond.Redirect(w, r, http.StatusFound, authCodeUrl)
		} else {
			respond.JSON(w, http.StatusOK, model.OauthInitiationResponse{
				RedirectUrl: authCodeUrl,
				State:       state,
			})
		}
	}
}

// handleOauthCallbackGet returns a gorm Handler function.
//
// @ID auth_handleOauthCallbackGet
// @Tags Authentication
// @Summary Handle the OAuth callback.
// @Produce json
// @Success 200 {object} []model.LoginProviderInfo
// @Router /auth/login/{provider}/callback [get]
func (e AuthEndpoint) handleOauthCallbackGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())

		var returnUrl *url.URL
		redirectToReturn := func(loginState string) {
			respond.Redirect(w, r, http.StatusFound, e.returnUrlWithLoginState(returnUrl, loginState))
		}

		if currentSession.OauthReturnTo != "" && e.isValidReturnUrl(currentSession.OauthReturnTo) {
			if u, err := url.Parse(currentSession.OauthReturnTo); err == nil {
				returnUrl = u
			}
		}

		if currentSession.LoggedIn {
			if returnUrl != nil {
				redirectToReturn("success")
			} else {
				respond.JSON(w, http.StatusBadRequest, model.Error{Message: "already logged in"})
			}
			return
		}

		provider := request.Path(r, "provider")
		oauthCode := request.Query(r, "code")
		oauthState := request.Query(r, "state")

		if provider != currentSession.OauthProvider {
			slog.Debug("invalid oauth provider in callback",
				"expected", currentSession.OauthProvider, "got", provider, "state", oauthState)
			if returnUrl != nil {
				redirectToReturn("err")
			} else {
				respond.JSON(w, http.StatusBadRequest,
					model.Error{Code: http.StatusBadRequest, Message: "invalid oauth provider"})
			}
			return
		}
		if oauthState != currentSession.OauthState {
			slog.Debug("invalid oauth state in callback",
				"expected", currentSession.OauthState, "got", oauthState, "provider", provider)
			if returnUrl != nil {
				redirectToReturn("err")
			} else {
				respond.JSON(w, http.StatusBadRequest,
					model.Error{Code: http.StatusBadRequest, Message: "invalid oauth state"})
			}
			return
		}

		loginCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // avoid long waits
		user, idTokenHint, err := e.authService.OauthLoginStep2(loginCtx, provider, currentSession.OauthNonce,
			oauthCode)
		cancel()
		if err != nil {
			slog.Debug("failed to process oauth code",
				"provider", provider, "state", oauthState, "error", err)
			if returnUrl != nil {
				redirectToReturn("err")
			} else {
				respond.JSON(w, http.StatusUnauthorized,
					model.Error{Code: http.StatusUnauthorized, Message: err.Error()})
			}
			return
		}

		e.setAuthenticatedUser(r, user, provider, idTokenHint)

		if returnUrl != nil {
			redirectToReturn("success")
		} else {
			respond.JSON(w, http.StatusOK, user)
		}
	}
}

func (e AuthEndpoint) setAuthenticatedUser(r *http.Request, user *domain.User, oauthProvider, idTokenHint string) {
	// Preserve the CSRF token across the session destroy: the SPA reuses the token
	// it fetched before login for subsequent requests until it reloads the page.
	csrfToken := e.session.GetData(r.Context()).CsrfToken

	// start a fresh session
	e.session.DestroyData(r.Context())

	currentSession := e.session.GetData(r.Context())

	currentSession.LoggedIn = true
	currentSession.IsAdmin = user.IsAdmin
	currentSession.UserIdentifier = string(user.Identifier)
	currentSession.Firstname = user.Firstname
	currentSession.Lastname = user.Lastname
	currentSession.Email = user.Email

	currentSession.OauthState = ""
	currentSession.OauthNonce = ""
	currentSession.OauthProvider = oauthProvider
	currentSession.OauthReturnTo = ""
	currentSession.OauthIdToken = idTokenHint
	currentSession.CsrfToken = csrfToken

	e.session.SetData(r.Context(), currentSession)
}

// needsTwoFactor reports whether the user must complete a TOTP second factor
// after the first factor. Only database-source users with opt-in 2FA qualify;
// OAuth/LDAP/OIDC users manage 2FA at their external provider.
func (e AuthEndpoint) needsTwoFactor(user *domain.User) bool {
	return e.totp != nil && user.TotpEnabled && user.CanUseTotp()
}

// currentUser loads the full user record for the logged-in session.
func (e AuthEndpoint) currentUser(r *http.Request) (*domain.User, error) {
	currentSession := e.session.GetData(r.Context())
	if !currentSession.LoggedIn || currentSession.UserIdentifier == "" {
		return nil, errors.New("not logged in")
	}
	return e.authService.GetUser(r.Context(), domain.UserIdentifier(currentSession.UserIdentifier))
}

// totpUser loads the user record for a pending-2FA session (not yet logged in).
func (e AuthEndpoint) totpUser(id domain.UserIdentifier) (*domain.User, error) {
	return e.authService.GetUser(context.Background(), id)
}

// clientIP returns the real client IP, honoring trusted proxies.
func clientIP(r *http.Request) string {
	return request.ClientIp(r, request.CheckPrivateProxy)
}

// setPendingTwoFactor puts the session into a pending-2FA state: the first factor
// succeeded, but the user is NOT logged in (LoggedIn=false) until the TOTP code is validated.
func (e AuthEndpoint) setPendingTwoFactor(r *http.Request, user *domain.User) {
	// Preserve the CSRF token across the session destroy: the SPA keeps using the
	// token it fetched before login for the follow-up /login/totp request.
	csrfToken := e.session.GetData(r.Context()).CsrfToken

	// start a fresh session, do not grant access
	e.session.DestroyData(r.Context())

	currentSession := e.session.GetData(r.Context())
	currentSession.LoggedIn = false
	currentSession.PendingTwoFactor = true
	currentSession.PendingTwoFactorUserId = string(user.Identifier)
	currentSession.CsrfToken = csrfToken
	e.session.SetData(r.Context(), currentSession)
}

// handleLoginTotpPost validates the TOTP (or recovery) code for a pending-2FA session
// and, on success, grants the full session. Rate limiting is enforced per user+IP.
//
// @ID auth_handleLoginTotpPost
// @Tags Authentication
// @Summary Validate the TOTP second factor after a successful first factor.
// @Produce json
// @Success 200 {object} model.LoginResponse
// @Router /auth/login/totp [post]
func (e AuthEndpoint) handleLoginTotpPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())
		if !currentSession.PendingTwoFactor || currentSession.PendingTwoFactorUserId == "" {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "no pending two-factor authentication"})
			return
		}

		var totpData struct {
			Code string `json:"code" binding:"required"`
		}
		if err := request.BodyJson(r, &totpData); err != nil {
			respond.JSON(w, http.StatusBadRequest, model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		// Rate limiting (per pending user + client IP). Critical against TOTP brute-force (AuthQuake).
		rateKey := currentSession.PendingTwoFactorUserId + "|" + clientIP(r)
		if !totpRateLimiter.allow(rateKey) {
			respond.JSON(w, http.StatusTooManyRequests,
				model.Error{Code: http.StatusTooManyRequests, Message: "too many attempts, try again later"})
			return
		}

		user, err := e.totpUser(domain.UserIdentifier(currentSession.PendingTwoFactorUserId))
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "invalid session"})
			return
		}

		if err := e.totp.ValidateCode(r.Context(), user, totpData.Code); err != nil {
			totpRateLimiter.recordFailure(rateKey)
			slog.Warn("totp validation failed", "user", currentSession.PendingTwoFactorUserId, "ip", clientIP(r))
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "invalid code"})
			return
		}

		// Success: clear rate-limit state and grant the full session.
		totpRateLimiter.reset(rateKey)
		e.setAuthenticatedUser(r, user, "", "")
		slog.Info("totp second factor success", "user", currentSession.PendingTwoFactorUserId)

		respond.JSON(w, http.StatusOK, model.LoginResponse{NeedTotp: false})
	}
}

// handleTotpStatusGet reports whether 2FA is available and enabled for the current user.
//
// @ID auth_handleTotpStatusGet
// @Tags Authentication
// @Summary Get TOTP 2FA status for the current user.
// @Produce json
// @Success 200 {object} model.TotpStatus
// @Router /auth/totp/status [get]
func (e AuthEndpoint) handleTotpStatusGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := e.currentUser(r)
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "not logged in"})
			return
		}
		respond.JSON(w, http.StatusOK, model.TotpStatus{
			Available: user.CanUseTotp(),
			Enabled:   user.TotpEnabled,
		})
	}
}

// handleTotpEnrollStart starts 2FA enrollment: generates a secret + QR and holds it in the session.
//
// @ID auth_handleTotpEnrollStart
// @Tags Authentication
// @Summary Start TOTP 2FA enrollment.
// @Produce json
// @Success 200 {object} model.TotpEnrollment
// @Router /auth/totp/enroll/start [post]
func (e AuthEndpoint) handleTotpEnrollStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := e.currentUser(r)
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "not logged in"})
			return
		}

		enrollment, err := e.totp.StartEnrollment(user)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		// Hold the pending secret in the session until confirmation (never logged).
		currentSession := e.session.GetData(r.Context())
		currentSession.PendingTotpSecret = enrollment.Secret
		e.session.SetData(r.Context(), currentSession)

		respond.JSON(w, http.StatusOK, model.TotpEnrollment{
			Secret:      enrollment.Secret,
			URL:         enrollment.URL,
			QRCodeImage: enrollment.QRCodeImage,
		})
	}
}

// handleTotpEnrollConfirm confirms enrollment with the first TOTP code and returns recovery codes.
//
// @ID auth_handleTotpEnrollConfirm
// @Tags Authentication
// @Summary Confirm TOTP 2FA enrollment and receive recovery codes.
// @Produce json
// @Success 200 {object} model.TotpRecoveryCodes
// @Router /auth/totp/enroll/confirm [post]
func (e AuthEndpoint) handleTotpEnrollConfirm() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := e.currentUser(r)
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "not logged in"})
			return
		}

		var confirmData struct {
			Code string `json:"code" binding:"required"`
		}
		if err := request.BodyJson(r, &confirmData); err != nil {
			respond.JSON(w, http.StatusBadRequest, model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		currentSession := e.session.GetData(r.Context())
		pendingSecret := currentSession.PendingTotpSecret
		if pendingSecret == "" {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "no pending enrollment, call enroll/start first"})
			return
		}

		codes, err := e.totp.ConfirmEnrollment(r.Context(), user, pendingSecret, confirmData.Code)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		// Clear the pending secret now that enrollment is complete.
		currentSession.PendingTotpSecret = ""
		e.session.SetData(r.Context(), currentSession)

		slog.Info("totp 2fa enabled", "user", string(user.Identifier))
		respond.JSON(w, http.StatusOK, model.TotpRecoveryCodes{Codes: codes})
	}
}

// handleTotpDisablePost disables 2FA for the current user.
//
// @ID auth_handleTotpDisablePost
// @Tags Authentication
// @Summary Disable TOTP 2FA for the current user.
// @Produce json
// @Success 200 {object} model.TotpStatus
// @Router /auth/totp/disable [post]
func (e AuthEndpoint) handleTotpDisablePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := e.currentUser(r)
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "not logged in"})
			return
		}

		if err := e.totp.Disable(r.Context(), user); err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		slog.Info("totp 2fa disabled", "user", string(user.Identifier))
		respond.JSON(w, http.StatusOK, model.TotpStatus{Available: user.CanUseTotp(), Enabled: false})
	}
}

// handleLoginPost returns a gorm Handler function.
//
// @ID auth_handleLoginPost
// @Tags Authentication
// @Summary Get all available external login providers.
// @Produce json
// @Success 200 {object} []model.LoginProviderInfo
// @Router /auth/login [post]
func (e AuthEndpoint) handleLoginPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())
		if currentSession.LoggedIn {
			respond.JSON(w, http.StatusOK, model.Error{Code: http.StatusOK, Message: "already logged in"})
			return
		}

		var loginData struct {
			Username string `json:"username" binding:"required,min=2"`
			Password string `json:"password" binding:"required,min=4"`
		}

		if err := request.BodyJson(r, &loginData); err != nil {
			respond.JSON(w, http.StatusBadRequest, model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}
		if err := e.validate.Struct(loginData); err != nil {
			respond.JSON(w, http.StatusBadRequest, model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		user, err := e.authService.PlainLogin(context.Background(), loginData.Username,
			loginData.Password)
		if err != nil {
			respond.JSON(w, http.StatusUnauthorized,
				model.Error{Code: http.StatusUnauthorized, Message: "login failed"})
			return
		}

		// Second factor required? Hold the session in a pending state until TOTP is validated.
		if e.needsTwoFactor(user) {
			e.setPendingTwoFactor(r, user)
			respond.JSON(w, http.StatusOK, model.LoginResponse{NeedTotp: true})
			return
		}

		e.setAuthenticatedUser(r, user, "", "")

		respond.JSON(w, http.StatusOK, user)
	}
}

// handleLogoutPost returns a gorm Handler function.
//
// @ID auth_handleLogoutPost
// @Tags Authentication
// @Summary Get all available external login providers.
// @Produce json
// @Success 200 {object} model.LogoutResponse
// @Router /auth/logout [post]
func (e AuthEndpoint) handleLogoutPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentSession := e.session.GetData(r.Context())

		if !currentSession.LoggedIn { // Not logged in
			respond.JSON(w, http.StatusOK, model.LogoutResponse{Message: "not logged in"})
			return
		}

		postLogoutRedirectUri := e.frontendUrl("/login")

		var redirectUrl *string
		if currentSession.OauthProvider != "" {
			if idpLogoutUrl, ok := e.authService.OauthProviderLogoutUrl(currentSession.OauthProvider,
				currentSession.OauthIdToken, postLogoutRedirectUri); ok {
				redirectUrl = &idpLogoutUrl
			}
		}

		e.session.DestroyData(r.Context())
		respond.JSON(w, http.StatusOK, model.LogoutResponse{Message: "logout ok", RedirectUrl: redirectUrl})
	}
}

// isValidReturnUrl checks if the given return URL matches the configured external URL of the application.
func (e AuthEndpoint) isValidReturnUrl(returnUrl string) bool {
	expectedUrl, err := url.Parse(e.cfg.Web.ExternalUrl)
	if err != nil {
		return false
	}

	returnUrlParsed, err := url.Parse(returnUrl)
	if err != nil {
		return false
	}

	if returnUrlParsed.Scheme != expectedUrl.Scheme || returnUrlParsed.Host != expectedUrl.Host {
		return false
	}

	if e.cfg.Web.BasePath != "" {
		expectedPath := e.cfg.Web.BasePath + "/app"
		if returnUrlParsed.Path != expectedPath && !strings.HasPrefix(returnUrlParsed.Path, expectedPath+"/") {
			return false
		}
	}

	return true
}

func (e AuthEndpoint) frontendUrl(route string) string {
	frontendUrl := e.cfg.Web.ExternalUrl + e.cfg.Web.BasePath + "/app/"
	if route != "" {
		frontendUrl += "#" + route
	}
	return frontendUrl
}

func (e AuthEndpoint) returnUrlWithLoginState(returnUrl *url.URL, loginState string) string {
	if returnUrl == nil {
		frontendURL, err := url.Parse(e.frontendUrl("/login"))
		if err != nil {
			return e.frontendUrl("/login")
		}
		returnUrl = frontendURL
	}

	redirectUrl := *returnUrl

	if redirectUrl.Fragment != "" {
		fragmentPath := redirectUrl.Fragment
		fragmentQuery := ""
		if queryStart := strings.Index(fragmentPath, "?"); queryStart >= 0 {
			fragmentQuery = fragmentPath[queryStart+1:]
			fragmentPath = fragmentPath[:queryStart]
		}

		queryParams, err := url.ParseQuery(fragmentQuery)
		if err != nil {
			queryParams = url.Values{}
		}
		queryParams.Set("wgLoginState", loginState)
		redirectUrl.Fragment = fragmentPath + "?" + queryParams.Encode()

		return redirectUrl.String()
	}

	queryParams := redirectUrl.Query()
	queryParams.Set("wgLoginState", loginState)
	redirectUrl.RawQuery = queryParams.Encode()

	return redirectUrl.String()
}

// handleWebAuthnCredentialsGet returns a gorm Handler function.
//
// @ID auth_handleWebAuthnCredentialsGet
// @Tags Authentication
// @Summary Get all available external login providers.
// @Produce json
// @Success 200 {object} []model.WebAuthnCredentialResponse
// @Router /auth/webauthn/credentials [get]
func (e AuthEndpoint) handleWebAuthnCredentialsGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusOK, []model.WebAuthnCredentialResponse{})
			return
		}

		currentSession := e.session.GetData(r.Context())

		userIdentifier := domain.UserIdentifier(currentSession.UserIdentifier)

		credentials, err := e.webAuthn.GetCredentials(r.Context(), userIdentifier)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		respond.JSON(w, http.StatusOK, model.NewWebAuthnCredentialResponses(credentials))
	}
}

// handleWebAuthnCredentialsDelete returns a gorm Handler function.
//
// @ID auth_handleWebAuthnCredentialsDelete
// @Tags Authentication
// @Summary Delete a WebAuthn credential.
// @Param id path string true "Base64 encoded Credential ID"
// @Produce json
// @Success 200 {object} []model.WebAuthnCredentialResponse
// @Router /auth/webauthn/credential/{id} [delete]
func (e AuthEndpoint) handleWebAuthnCredentialsDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		currentSession := e.session.GetData(r.Context())

		userIdentifier := domain.UserIdentifier(currentSession.UserIdentifier)

		credentialId := Base64UrlDecode(request.Path(r, "id"))

		credentials, err := e.webAuthn.RemoveCredential(r.Context(), userIdentifier, credentialId)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		respond.JSON(w, http.StatusOK, model.NewWebAuthnCredentialResponses(credentials))
	}
}

// handleWebAuthnCredentialsPut returns a gorm Handler function.
//
// @ID auth_handleWebAuthnCredentialsPut
// @Tags Authentication
// @Summary Update a WebAuthn credential.
// @Param id path string true "Base64 encoded Credential ID"
// @Param request body model.WebAuthnCredentialRequest true "Credential name"
// @Produce json
// @Success 200 {object} []model.WebAuthnCredentialResponse
// @Router /auth/webauthn/credential/{id} [put]
func (e AuthEndpoint) handleWebAuthnCredentialsPut() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		currentSession := e.session.GetData(r.Context())

		userIdentifier := domain.UserIdentifier(currentSession.UserIdentifier)

		credentialId := Base64UrlDecode(request.Path(r, "id"))
		var req model.WebAuthnCredentialRequest
		if err := request.BodyJson(r, &req); err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		credentials, err := e.webAuthn.UpdateCredential(r.Context(), userIdentifier, credentialId, req.Name)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		respond.JSON(w, http.StatusOK, model.NewWebAuthnCredentialResponses(credentials))
	}
}

func (e AuthEndpoint) handleWebAuthnRegisterStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		currentSession := e.session.GetData(r.Context())

		userIdentifier := domain.UserIdentifier(currentSession.UserIdentifier)

		options, sessionData, err := e.webAuthn.StartWebAuthnRegistration(r.Context(), userIdentifier)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		currentSession.WebAuthnData = string(sessionData)
		e.session.SetData(r.Context(), currentSession)

		respond.Data(w, http.StatusOK, "application/json", options)
	}
}

// handleWebAuthnRegisterFinish returns a gorm Handler function.
//
// @ID auth_handleWebAuthnRegisterFinish
// @Tags Authentication
// @Summary Finish the WebAuthn registration process.
// @Param credential_name query string false "Credential name" default("")
// @Produce json
// @Success 200 {object} []model.WebAuthnCredentialResponse
// @Router /auth/webauthn/register/finish [post]
func (e AuthEndpoint) handleWebAuthnRegisterFinish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		name := request.QueryDefault(r, "credential_name", "")

		currentSession := e.session.GetData(r.Context())

		webAuthnSessionData := []byte(currentSession.WebAuthnData)
		currentSession.WebAuthnData = "" // clear the session data
		e.session.SetData(r.Context(), currentSession)

		credentials, err := e.webAuthn.FinishWebAuthnRegistration(
			r.Context(),
			domain.UserIdentifier(currentSession.UserIdentifier),
			name,
			webAuthnSessionData,
			r)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		respond.JSON(w, http.StatusOK, model.NewWebAuthnCredentialResponses(credentials))
	}
}

func (e AuthEndpoint) handleWebAuthnLoginStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		currentSession := e.session.GetData(r.Context())

		options, sessionData, err := e.webAuthn.StartWebAuthnLogin(r.Context())
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		currentSession.WebAuthnData = string(sessionData)
		e.session.SetData(r.Context(), currentSession)

		respond.Data(w, http.StatusOK, "application/json", options)
	}
}

// handleWebAuthnLoginFinish returns a gorm Handler function.
//
// @ID auth_handleWebAuthnLoginFinish
// @Tags Authentication
// @Summary Finish the WebAuthn login process.
// @Produce json
// @Success 200 {object} model.User
// @Router /auth/webauthn/login/finish [post]
func (e AuthEndpoint) handleWebAuthnLoginFinish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !e.webAuthn.Enabled() {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: "WebAuthn is not enabled"})
			return
		}

		currentSession := e.session.GetData(r.Context())

		webAuthnSessionData := []byte(currentSession.WebAuthnData)
		currentSession.WebAuthnData = "" // clear the session data
		e.session.SetData(r.Context(), currentSession)

		user, err := e.webAuthn.FinishWebAuthnLogin(
			r.Context(),
			webAuthnSessionData,
			r)
		if err != nil {
			respond.JSON(w, http.StatusBadRequest,
				model.Error{Code: http.StatusBadRequest, Message: err.Error()})
			return
		}

		// Second factor required? Hold the session in a pending state until TOTP is validated.
		if e.needsTwoFactor(user) {
			e.setPendingTwoFactor(r, user)
			respond.JSON(w, http.StatusOK, model.LoginResponse{NeedTotp: true})
			return
		}

		e.setAuthenticatedUser(r, user, "", "")

		respond.JSON(w, http.StatusOK, model.NewUser(user, false))
	}
}
