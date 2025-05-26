package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gisquick/gisquick-server/internal/domain"
	"github.com/go-redis/redis/v8"
	"github.com/gofrs/uuid"
	"github.com/jellydator/ttlcache/v3"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

var (
	ErrUserNotFound    = errors.New("User not found")
	ErrInvalidPassword = errors.New("Password doesn't match")
	ErrInvalidSession  = errors.New("Invalid session")
	AnonymousUser      = domain.User{IsGuest: true}
)

const (
	basic = "basic"
)

type SessionInfo struct {
	ID       string
	Username string
}

type SessionStore interface {
	Set(ctx context.Context, sessionID, data string, expiration time.Duration) error
	Get(ctx context.Context, sessionID string) (string, error)
	Del(ctx context.Context, sessionID string) error
}

type RedisSessionStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{rdb: rdb}
}

func (s *RedisSessionStore) Set(ctx context.Context, sessionID, data string, expiration time.Duration) error {
	if err := s.rdb.Set(ctx, sessionID, data, expiration).Err(); err != nil {
		return fmt.Errorf("redis save session: %v", err)
	}
	return nil
}

func (s *RedisSessionStore) Get(ctx context.Context, sessionID string) (string, error) {
	val, err := s.rdb.Get(ctx, sessionID).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrInvalidSession
		}
		return "", fmt.Errorf("redis get session: %v", err)
	}
	return val, nil
}

func (s *RedisSessionStore) Del(ctx context.Context, sessionID string) error {
	if err := s.rdb.Del(ctx, sessionID).Err(); err != nil {
		return fmt.Errorf("redis delete session: %v", err)
	}
	return nil
}

type AuthService struct {
	logger         *zap.SugaredLogger
	expiration     time.Duration
	accounts       domain.AccountsRepository
	store          SessionStore
	cache          *ttlcache.Cache[string, domain.User]
	basicAuthCache *ttlcache.Cache[string, domain.User]
}

func NewAuthService(logger *zap.SugaredLogger, expiration time.Duration, accounts domain.AccountsRepository, store SessionStore) *AuthService {
	loader := ttlcache.LoaderFunc[string, domain.User](
		func(c *ttlcache.Cache[string, domain.User], username string) *ttlcache.Item[string, domain.User] {
			account, err := accounts.GetByUsername(username)
			if err != nil {
				logger.Errorw("getting account", "username", username, zap.Error(err))
				return nil
			}
			item := c.Set(username, domain.AccountToUser(account), ttlcache.DefaultTTL)
			return item
		},
	)
	cache := ttlcache.New(
		ttlcache.WithTTL[string, domain.User](45*time.Second),
		ttlcache.WithLoader[string, domain.User](loader),
		ttlcache.WithDisableTouchOnHit[string, domain.User](),
	)

	basicAuthCache := ttlcache.New(
		ttlcache.WithTTL[string, domain.User](45*time.Second),
		ttlcache.WithDisableTouchOnHit[string, domain.User](),
	)
	return &AuthService{
		logger:         logger,
		expiration:     expiration,
		accounts:       accounts,
		store:          store,
		cache:          cache,
		basicAuthCache: basicAuthCache,
	}
}

func (s *AuthService) GetSessionInfo(c echo.Context) (*SessionInfo, error) {
	si, saved := c.Get("session").(SessionInfo)
	if saved {
		return &si, nil
	}
	sessionid := ""
	cookie, err := c.Request().Cookie("gq_session")
	if err == nil {
		sessionid = cookie.Value
	}
	if sessionid == "" {
		c.Set("session", nil)
		return nil, nil
	}
	data, err := s.store.Get(c.Request().Context(), sessionid)
	if err != nil {
		if errors.Is(err, ErrInvalidSession) {
			s.LogoutUser(c)
			c.Set("session", nil)
			return nil, nil
		}
		return nil, err
	}
	si = SessionInfo{ID: sessionid, Username: data}
	c.Set("session", si)
	return &si, nil
}

func (s *AuthService) GetUser(c echo.Context) (domain.User, error) {
	user, saved := c.Get("user").(domain.User)
	if saved {
		return user, nil
	}
	auth := c.Request().Header.Get("Authorization")
	if auth != "" {
		if item := s.basicAuthCache.Get(auth); item != nil {
			user = item.Value()
		} else {
			prefixLen := len(basic)
			if len(auth) > prefixLen+1 && strings.EqualFold(auth[:prefixLen], basic) {
				b, err := base64.StdEncoding.DecodeString(auth[prefixLen+1:])
				if err != nil {
					return AnonymousUser, err
				}
				cred := strings.SplitN(string(b), ":", 2)
				if len(cred) == 2 {
					account, err := s.Authenticate(cred[0], cred[1])
					if err != nil {
						return AnonymousUser, err
					}
					user = domain.AccountToUser(account)
					s.basicAuthCache.Set(auth, user, ttlcache.DefaultTTL)
				}
			}
		}
	} else {
		session, err := s.GetSessionInfo(c)
		if err != nil {
			return AnonymousUser, fmt.Errorf("auth: get session user: %w", err)
		}
		if session == nil {
			return AnonymousUser, nil
		}
		item := s.cache.Get(session.Username)
		if item == nil {
			return AnonymousUser, nil
		}
		user = item.Value()
	}
	c.Set("user", user)
	return user, nil
}

func (s *AuthService) Authenticate(login, password string) (domain.Account, error) {
	var account domain.Account
	var err error
	if strings.Contains(login, "@") {
		account, err = s.accounts.GetByEmail(login)
	} else {
		account, err = s.accounts.GetByUsername(login)
	}
	if err != nil {
		return domain.Account{}, err
	}
	if !account.Active {
		return domain.Account{}, ErrUserNotFound
	}
	if !account.CheckPassword(password) {
		return domain.Account{}, ErrInvalidPassword
	}
	return account, nil
}

func (s *AuthService) LoginUserWithExpiration(c echo.Context, userAccount domain.Account, expiration time.Duration) error {
	token, err := uuid.NewV4()
	if err != nil {
		return err
	}
	sessionid := token.String()
	// sessionid := fmt.Sprintf("%s:%s", user.Username, token.String())
	if err := s.store.Set(c.Request().Context(), sessionid, userAccount.Username, expiration); err != nil {
		return fmt.Errorf("save session: %v", err)
	}
	oldCookie, err := c.Request().Cookie("gq_session")
	if err == nil {
		if err = s.store.Del(c.Request().Context(), oldCookie.Value); err != nil {
			s.logger.Errorw("deleting old session on login", zap.Error(err))
		}
	}
	now := time.Now().UTC()
	userAccount.LastLogin = &now
	if err := s.accounts.Update(userAccount); err != nil {
		s.logger.Warnw("updating time of last login", zap.Error(err))
	}

	// serverUrl.Hostname()
	// c.Request().URL.Hostname()
	http.SetCookie(c.Response(), &http.Cookie{
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Name:     "gq_session",
		Value:    sessionid,
		HttpOnly: true,
		Expires:  time.Now().Add(expiration),
	})
	return nil
}

func (s *AuthService) LoginUser(c echo.Context, userAccount domain.Account) error {
	return s.LoginUserWithExpiration(c, userAccount, s.expiration)
}

func (s *AuthService) LogoutUser(c echo.Context) {
	cookie, err := c.Request().Cookie("gq_session")
	if err == nil {
		if err = s.store.Del(c.Request().Context(), cookie.Value); err != nil {
			s.logger.Errorw("deleting session on logout", zap.Error(err))
		}
	}
	http.SetCookie(c.Response(), &http.Cookie{
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Name:     "gq_session",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
