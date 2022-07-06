package aad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	msalErrors "github.com/AzureAD/microsoft-authentication-library-for-go/apps/errors"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/ubuntu/aad-auth/internal/logger"
)

const (
	endpoint = "https://login.microsoftonline.com"

	invalidCredCode = 50126
	requiresMFACode = 50076
	noSuchUserCode  = 50034
)

var (
	// ErrNoNetwork is returned in case of no network available.
	ErrNoNetwork = errors.New("NO NETWORK")
	// ErrDeny is returned in case of denial returned by AAD.
	ErrDeny = errors.New("DENY")
)

type aadErr struct {
	ErrorCodes []int `json:"error_codes"`
}

// Authenticate tries to authenticate username against AAD.
func Authenticate(ctx context.Context, tenantID, appID, username, password string) error {
	authority := fmt.Sprintf("%s/%s", endpoint, tenantID)
	logger.Debug(ctx, "Connecting to %q, with clientID %q for user %q", authority, appID, username)

	// Get client from network
	app, errAcquireToken := public.New(appID, public.WithAuthority(authority))
	if errAcquireToken != nil {
		logger.Err(ctx, "Connection to authority failed: %v", errAcquireToken)
		return ErrNoNetwork
	}

	// Authentify the user
	_, errAcquireToken = app.AcquireTokenByUsernamePassword(context.Background(), nil, username, password)

	var callErr msalErrors.CallErr
	if errors.As(errAcquireToken, &callErr) {
		data, err := io.ReadAll(callErr.Resp.Body)
		if err != nil {
			logger.Err(ctx, "Can't read server response: %v", err)
			return ErrDeny
		}
		var addErrWithCodes aadErr
		if err := json.Unmarshal(data, &addErrWithCodes); err != nil {
			logger.Err(ctx, "Invalid server response, not a json object: %v", err)
			return ErrDeny
		}
		for _, errcode := range addErrWithCodes.ErrorCodes {
			if errcode == invalidCredCode {
				logger.Debug(ctx, "Got response: Invalid credentials")
				return ErrDeny
			}
			if errcode == noSuchUserCode {
				logger.Debug(ctx, "Got response: User doesn't exist")
				return ErrDeny
			}
			if errcode == requiresMFACode {
				logger.Debug(ctx, "Authentication successful even if requiring MFA")
				return nil
			}
		}
		logger.Err(ctx, "Unknown error code(s) from server: %v", addErrWithCodes.ErrorCodes)
		return ErrDeny
	}

	if errAcquireToken != nil {
		logger.Debug(ctx, "acquiring token failed: %v", errAcquireToken)
		return ErrNoNetwork
	}

	logger.Debug(ctx, "Authentication successful with user/password")
	return nil
}
