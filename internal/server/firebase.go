//go:build firebase
// +build firebase

package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	firebase "firebase.google.com/go/v4"
	firebaseAuth "firebase.google.com/go/v4/auth"
	"github.com/gisquick/gisquick-server/internal/server/auth"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

func (s *Server) handleFirebaseLogin(client *firebaseAuth.Client) func(echo.Context) error {
	type Payload struct {
		Token string `json:"idToken"`
	}
	return func(c echo.Context) error {
		form := new(Payload)
		if err := c.Bind(form); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		// v1
		/*
			expiresIn := time.Hour * 2
			cookie, err := client.SessionCookie(c.Request().Context(), form.Token, expiresIn)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create a session cookie")
			}
			http.SetCookie(c.Response(), &http.Cookie{
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
				Name:     "fb_session",
				Value:    cookie,
				MaxAge:   int(expiresIn.Seconds()),
				HttpOnly: true,
				Secure:   true,
			})
		*/
		// v2
		token, err := client.VerifyIDToken(c.Request().Context(), form.Token)
		if err != nil {
			s.log.Warnw("error verifying Firebase ID token", zap.Error(err))
			return echo.NewHTTPError(http.StatusUnauthorized, "Please provide valid credentials")
		}
		email := token.Claims["email"].(string)
		account, err := s.accountsService.Repository.GetByEmail(email)
		expiration := 62 * time.Minute
		if err := s.auth.LoginUserWithExpiration(c, account, expiration); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, auth.AccountToUser(account))
	}
}

func init() {
	extensions["firebase"] = func(s *Server) error {
		// todo: create and parse Firebase config from environment variables
		opt := option.WithCredentialsFile("/etc/gisquick/firebase/serviceAccountKey.json")
		app, err := firebase.NewApp(context.Background(), nil, opt)
		if err != nil {
			return fmt.Errorf("initializing firebase app: %w", err)
		}
		client, err := app.Auth(context.Background())
		if err != nil {
			return fmt.Errorf("initializing firebase client: %w", err)
		}

		s.echo.POST("/api/auth/firebase/login", s.handleFirebaseLogin(client))
		return nil
	}
}
