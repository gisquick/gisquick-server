package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/gisquick/gisquick-server/internal/domain"
	"github.com/jackc/pgconn"
	"github.com/jmoiron/sqlx"
)

type AccountsRepository struct {
	db *sqlx.DB
}

func NewAccountsRepository(db *sqlx.DB) *AccountsRepository {
	return &AccountsRepository{db}
}

func (r *AccountsRepository) Create(account domain.Account) error {
	dbUser := toUser(account)
	_, err := r.db.NamedExec(
		`INSERT INTO users (username, email, password, first_name, last_name, is_superuser, is_active, created_at, confirmed_at, last_login_at)
		VALUES (:username, :email, :password, :first_name, :last_name, :is_superuser, :is_active, :created_at, :confirmed_at, :last_login_at)`,
		&dbUser,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // UniqueViolation
			return domain.ErrAccountExists
		}
		// for 'postgres' driver
		// if err, ok := err.(*pq.Error); ok {
		// 	log.Println("PG ERROR Code #2:", err.Code)
		// }
		return err
	}
	return nil
}

func (r *AccountsRepository) Delete(username string) error {
	_, err := r.db.Exec("DELETE FROM users WHERE username=$1", username)
	return err
}

func (r *AccountsRepository) find(q string, args ...interface{}) (domain.Account, error) {
	var user User
	err := r.db.Get(&user, q, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Account{}, domain.ErrAccountNotFound
		}
		return domain.Account{}, err
	}
	return toAccount(user), nil
}

func (r *AccountsRepository) GetByUsername(username string) (domain.Account, error) {
	account, err := r.find("SELECT * FROM users WHERE username=$1", username)
	return account, err
}

func (r *AccountsRepository) GetByEmail(email string) (domain.Account, error) {
	var dbUsers []User
	err := r.db.Select(&dbUsers, `SELECT * FROM users WHERE email LIKE $1`, email)
	if err != nil {
		return domain.Account{}, err
	}
	if len(dbUsers) == 0 {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if len(dbUsers) > 1 {
		return domain.Account{}, fmt.Errorf("More than 1 accounts with email address")
	}
	return toAccount(dbUsers[0]), nil
}

func (r *AccountsRepository) Update(account domain.Account) error {
	user := toUser(account)
	const q = `
	UPDATE
			users
	SET
			"username" = :username,
			"email" = :email,
			"password" = :password,
			"first_name" = :first_name,
			"last_name" = :last_name,
			"is_superuser" = :is_superuser,
			"is_active" = :is_active,
			"created_at" = :created_at,
			"confirmed_at" = :confirmed_at,
			"last_login_at" = :last_login_at
	WHERE
			username = :username
	`
	_, err := r.db.NamedExec(q, user)
	return err
}

func (r *AccountsRepository) UpdateProfile(account domain.Account) error {
	user := toUser(account)
	const q = `UPDATE users SET "profile" = :profile WHERE username = :username`
	_, err := r.db.NamedExec(q, user)
	return err
}

func (r *AccountsRepository) UpdateProfile2(username string, profile domain.Profile) error {
	const q = `UPDATE users SET "profile" = :profile WHERE username = :username`
	user := domain.User{Username: username, Profile: profile}
	_, err := r.db.NamedExec(q, user)
	return err
}

func (r *AccountsRepository) EmailExists(email string) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT exists (SELECT 1 FROM users WHERE email ILIKE $1)", email).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return exists, nil
}

func (r *AccountsRepository) UsernameExists(username string) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT exists (SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return exists, nil
}

// func (r *AccountsRepository) ActivateAccount(username string) error {
// 	user := User{
// 		Username: username,
// 		IsActive: true,
// 	}
// 	_, err := r.db.NamedExec(`UPDATE users SET is_active=:is_active WHERE username=:username `, user)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

func (r *AccountsRepository) GetActiveAccounts() ([]domain.Account, error) {
	var dbUsers []User
	// err := r.db.Select(&dbUsers, `SELECT username, email, first_name, last_name, is_active, is_superuser, date_joined, last_login FROM users WHERE is_active=true`)
	err := r.db.Select(&dbUsers, `SELECT * FROM users WHERE is_active=true`)
	if err != nil {
		return nil, err
	}
	accounts := make([]domain.Account, len(dbUsers))
	for index, user := range dbUsers {
		accounts[index] = toAccount(user)
	}
	return accounts, nil
}

func (r *AccountsRepository) GetAllAccounts() ([]domain.Account, error) {
	var dbUsers []User
	// udb := r.db.Unsafe()
	// err := udb.Select(&dbUsers, `SELECT * FROM users`)
	err := r.db.Select(&dbUsers, `SELECT * FROM users`)
	if err != nil {
		return nil, err
	}
	accounts := make([]domain.Account, len(dbUsers))
	for index, user := range dbUsers {
		accounts[index] = toAccount(user)
	}
	return accounts, nil
}

func toAccount(user User) domain.Account {
	return domain.Account{
		Username:  user.Username,
		Email:     user.Email,
		Password:  user.Password,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Active:    user.IsActive,
		Superuser: user.IsSuperuser,
		Created:   user.Created,
		Confirmed: user.Confirmed,
		LastLogin: user.LastLogin,
		Profile:   domain.Profile(user.Profile),
	}
}

func toUser(a domain.Account) User {
	return User{
		Username:    a.Username,
		Email:       a.Email,
		Password:    a.Password,
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		IsActive:    a.Active,
		IsSuperuser: a.Superuser,
		Created:     a.Created,
		Confirmed:   a.Confirmed,
		LastLogin:   a.LastLogin,
		Profile:     UserProfile(a.Profile),
	}
}
