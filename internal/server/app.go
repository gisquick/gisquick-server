package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gisquick/gisquick-server/internal/domain"
	"github.com/gisquick/gisquick-server/internal/infrastructure/cache"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type AppConfig struct {
	Language       string `json:"lang,omitempty"`
	LandingProject string `json:"landing_project,omitempty"`
	Languages      []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"languages,omitempty"`
}

type AppData struct {
	AppConfig
	PasswordResetUrl string `json:"reset_password_url,omitempty"`
}

type UserInfo struct {
	Username  string         `json:"username"`
	FirstName string         `json:"first_name"`
	LastName  string         `json:"last_name"`
	FullName  string         `json:"full_name"`
	Active    bool           `json:"active"`
	Profile   map[string]any `json:"profile,omitempty"`
}

type UserData struct {
	domain.User
	Profile map[string]interface{} `json:"profile,omitempty"`
}

type AppPayload struct {
	App  AppData  `json:"app"`
	User UserData `json:"user"`
}

func (s *Server) getUserProfile(user domain.User) (map[string]interface{}, error) {
	var userProfile map[string]interface{}
	if user.IsAuthenticated {
		dashboardPath := filepath.Join(s.Config.ProjectsRoot, user.Username, "profile.json")
		content, err := os.ReadFile(dashboardPath)
		if err == nil {
			if err = json.Unmarshal(content, &userProfile); err != nil {
				return nil, fmt.Errorf("parsing user profile file: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading user profile file: %w", err)
		}
	}
	return userProfile, nil
}

func (s *Server) handleAppInit() func(echo.Context) error {
	configReader := cache.NewJSONFileReader[AppConfig](time.Hour)
	s.OnShutdown(configReader.Close)

	return func(c echo.Context) error {
		user, err := s.auth.GetUser(c)
		if err != nil {
			return fmt.Errorf("handleAppInit get user: %w", err)
		}
		// userdtoUser()
		config := AppConfig{
			Language:       s.Config.Language,
			LandingProject: s.Config.LandingProject,
		}
		userProfile, err := s.getUserProfile(user)
		if err != nil {
			s.log.Warnw("handleAppInit", "user", user.Username, zap.Error(err))
		}
		config, err = configReader.Extend("/etc/gisquick/app.json", config)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Errorw("reading app configuration file", zap.Error(err))
		}
		app := AppData{
			AppConfig: config,
		}
		if s.accountsService.SupportEmails() {
			app.PasswordResetUrl = "/api/accounts/password_reset"
		}
		data := AppPayload{
			App:  app,
			User: UserData{User: user, Profile: userProfile},
		}
		return c.JSON(http.StatusOK, data)
	}
}

type SessionData struct {
	User domain.User `json:"user"`
}

func (s *Server) handleGetSessionUser(c echo.Context) error {
	user, err := s.auth.GetUser(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, SessionData{user})
}

func (s *Server) handleGetUsers(c echo.Context) error {
	accounts, err := s.accountsService.GetAllAccounts()
	if err != nil {
		return err
	}
	res := []UserInfo{}
	for _, u := range accounts {
		res = append(res, UserInfo{
			Username:  u.Username,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			FullName:  u.FullName(),
			Active:    u.Active,
			Profile:   u.Profile,
		})
	}
	return c.JSON(http.StatusOK, res)
}
