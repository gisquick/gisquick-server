package postgres

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type UserProfile map[string]any

func (pc *UserProfile) Scan(val any) error {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, &pc)
		return nil
	case string:
		json.Unmarshal([]byte(v), &pc)
		return nil
	default:
		return fmt.Errorf("unsupported type: %T", v) //errors.New(fmt.Sprintf("Unsupported type: %T", v))
	}
}
func (pc *UserProfile) Value() (driver.Value, error) {
	return json.Marshal(pc)
}

type User struct {
	Username    string      `db:"username"`
	Email       string      `db:"email"`
	Password    []byte      `db:"password"`
	FirstName   string      `db:"first_name"`
	LastName    string      `db:"last_name"`
	IsSuperuser bool        `db:"is_superuser"`
	IsActive    bool        `db:"is_active"`
	Created     *time.Time  `db:"created_at"`
	Confirmed   *time.Time  `db:"confirmed_at"`
	LastLogin   *time.Time  `db:"last_login_at"`
	Profile     UserProfile `db:"profile"`
}
