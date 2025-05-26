package server

import (
	"net/http"

	"github.com/gisquick/gisquick-server/internal/domain"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (s *Server) handleLogin() func(echo.Context) error {
	type LoginForm struct {
		Username string `json:"username" form:"username" validate:"required"`
		Password string `json:"password" form:"password" validate:"required"`
	}
	var validate = validator.New()
	return func(c echo.Context) error {
		form := new(LoginForm)
		if err := c.Bind(form); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if err := validate.Struct(form); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		account, err := s.auth.Authenticate(form.Username, form.Password)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "Please provide valid credentials")
		}
		if err := s.auth.LoginUser(c, account); err != nil {
			return err
		}
		user := domain.AccountToUser(account)
		if user.Profile == nil {
			profile, err := s.getUserProfile(user)
			if err != nil {
				s.log.Warnw("handleLogin", "user", user.Username, zap.Error(err))
			}
			user.Profile = profile
		}
		return c.JSON(http.StatusOK, user)
	}
}

func (s *Server) handleLogout(c echo.Context) error {
	s.auth.LogoutUser(c)
	return c.NoContent(http.StatusOK)
}
